/*
 * Licensed to Elasticsearch B.V. under one or more contributor
 * license agreements. See the NOTICE file distributed with
 * this work for additional information regarding copyright
 * ownership. Elasticsearch B.V. licenses this file to you under
 * the Apache License, Version 2.0 (the "License"); you may
 * not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *	http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 *
 * This project is based on a modification of
 * https://github.com/grafana/xk6-output-prometheus-remote which
 * is licensed under the Apache 2.0 License.
 *
 */

package esoutput

import (
	"bytes"
	"context"
	"crypto/tls"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	es "github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esutil"
	"github.com/sirupsen/logrus"
	"go.k6.io/k6/output"
)

type elasticMetricEntry struct {
	MetricName string
	MetricType string
	Value      float64
	Tags       map[string]string
	Time       time.Time
}

type Output struct {
	config Config

	client          *es.Client
	bulkIndexer     esutil.BulkIndexer
	periodicFlusher *output.PeriodicFlusher
	output.SampleBuffer

	logger logrus.FieldLogger
}

const hasPrivilegesBody = `{
  "index": [
    {
      "names": [
        "%s"
      ],
      "privileges": [
        "write", "create_index"
      ]
    }
  ]
}`

var _ output.Output = new(Output)

//go:embed mapping.json
var mapping []byte

func New(params output.Params) (output.Output, error) {
	params.Logger.Info("Elasticsearch: configuring output")

	config, err := GetConsolidatedConfig(params.JSONConfig, params.Environment, params.ConfigArgument)
	if err != nil {
		return nil, err
	}

	var addresses = []string{config.Url.ValueOrZero()}

	var esConfig es.Config

	// Cloud id takes precedence over a URL (which is localhost by default)
	if config.CloudID.Valid {
		esConfig.CloudID = config.CloudID.String
	} else if config.Url.Valid {
		esConfig.Addresses = strings.Split(strings.Join(addresses, ""), ",")
	}
	if config.User.Valid {
		esConfig.Username = config.User.String
	}
	if config.Password.Valid {
		esConfig.Password = config.Password.String
	}
	if config.APIKey.Valid {
		esConfig.APIKey = config.APIKey.String
	}
	if config.ServiceAccountToken.Valid {
		esConfig.ServiceToken = config.ServiceAccountToken.String
	}
	if config.CACert.Valid {
		cert, err := os.ReadFile(config.CACert.String)
		if err != nil {
			return nil, err
		}
		esConfig.CACert = cert
	}

	var clientTLSCert tls.Certificate
	if config.ClientCert.Valid && config.ClientKey.Valid {
		clientTLSCert, err = tls.LoadX509KeyPair(config.ClientCert.String, config.ClientKey.String)
		if err != nil {
			return nil, err
		}
	}

	esConfig.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: config.InsecureSkipVerify.Bool,
			Certificates:       []tls.Certificate{clientTLSCert},
		},
	}

	client, err := es.NewClient(esConfig)
	if err != nil {
		return nil, err
	}
	// ensure basic connectivity
	info, err := client.Info()
	if err != nil {
		return nil, err
	}
	if info.StatusCode != 200 {
		// The info API requires the 'monitor' privilege and the user might not have that. We can only get a 403 if
		// security is configured on this cluster. Therefore, we call the has privilege API that is guaranteed to work
		//for every user.
		if info.StatusCode == 403 {
			priv, err := client.Security.HasPrivileges(strings.NewReader(fmt.Sprintf(hasPrivilegesBody, config.IndexName.String)))
			if err != nil {
				return nil, err
			}
			if priv.StatusCode != 200 {
				return nil, fmt.Errorf("cannot connect to Elasticsearch (status code %d)", priv.StatusCode)
			}
		} else {
			return nil, fmt.Errorf("cannot connect to Elasticsearch (status code %d)", info.StatusCode)
		}
	}

	bulkIndexer, err := esutil.NewBulkIndexer(esutil.BulkIndexerConfig{
		Index:  config.IndexName.String,
		Client: client,
		OnError: func(ctx context.Context, err error) {
			// this happens usually due to permission issues
			params.Logger.Errorf("Could not write metrics: %s", err)
		},
	})
	if err != nil {
		return nil, fmt.Errorf("error creating the indexer: %v", err)
	}

	return &Output{
		client:      client,
		bulkIndexer: bulkIndexer,
		config:      config,
		logger:      params.Logger,
	}, nil
}

func (*Output) Description() string {
	return "Output k6 metrics to Elasticsearch"
}

func (o *Output) Start() error {
	indexName := o.config.IndexName.String
	res, err := o.client.Indices.Create(indexName, o.client.Indices.Create.WithBody(bytes.NewReader(mapping)))
	if err != nil {
		return err
	}
	// 400 usually happens when the index already exists, which is ok for our purposes.
	if res.StatusCode > 400 {
		body, err := io.ReadAll(res.Body)
		if err != nil {
			return fmt.Errorf("could not read response after failure to create index %s: %v", indexName, err)
		}
		return fmt.Errorf("could not create index %s: %s", indexName, body)
	}
	res.Body.Close()

	if periodicFlusher, err := output.NewPeriodicFlusher(time.Duration(o.config.FlushPeriod.Duration), o.flush); err != nil {
		return err
	} else {
		o.periodicFlusher = periodicFlusher
	}
	o.logger.Debugf("Elasticsearch: starting writing to index %s", indexName)

	return nil
}

func (o *Output) Stop() error {
	o.logger.Debug("Elasticsearch: stopping writing")
	o.periodicFlusher.Stop()
	if err := o.bulkIndexer.Close(context.Background()); err != nil {
		log.Fatalf("Elasticsearch: Could not close bulk indexer: %s", err)
	}
	return nil
}

func (o *Output) blkItemErrHandler(ctx context.Context, item esutil.BulkIndexerItem, res esutil.BulkIndexerResponseItem, err error) {
	if err != nil {
		o.logger.Errorf("%s", err)
	} else {
		o.logger.Errorf("%s: %s", res.Error.Type, res.Error.Reason)
	}
}

func (o *Output) flush() {
	samplesContainers := o.GetBufferedSamples()
	for _, samplesContainer := range samplesContainers {
		samples := samplesContainer.GetSamples()

		for _, sample := range samples {
			mappedEntry := elasticMetricEntry{
				MetricName: sample.Metric.Name,
				MetricType: sample.Metric.Type.String(),
				Value:      sample.Value,
				Tags:       sample.GetTags().Map(),
				Time:       sample.Time,
			}
			data, err := json.Marshal(mappedEntry)
			if err != nil {
				o.logger.Fatalf("Cannot encode document: %s, %s", err, mappedEntry)
			}
			var item = esutil.BulkIndexerItem{
				Action:    "create",
				Body:      bytes.NewReader(data),
				OnFailure: o.blkItemErrHandler,
			}
			err = o.bulkIndexer.Add(
				context.Background(),
				item,
			)
			if err != nil {
				log.Fatalf("Unexpected error: %s", err)
			}
		}
	}
}
