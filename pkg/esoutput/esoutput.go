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
	_ "embed"
	"encoding/json"
	"log"
	"time"

	es "github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esutil"
	"github.com/sirupsen/logrus"
	"go.k6.io/k6/output"
)

type elasticMetricEntry struct {
	MetricName  string
	MetricType  string
	MetricValue float64
	MetricTags  map[string]string
	SampleTags  map[string]string
	Time        time.Time
}

type Output struct {
	config Config

	client          *es.Client
	bulkIndexer     esutil.BulkIndexer
	periodicFlusher *output.PeriodicFlusher
	output.SampleBuffer

	logger logrus.FieldLogger
}

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

	// Cloud id takes precendence over a URL (which is localhost by default)
	if config.CloudID.Valid {
		esConfig.CloudID = config.CloudID.String
	} else if config.Url.Valid {
		esConfig.Addresses = addresses
	}
	if config.User.Valid {
		esConfig.Username = config.User.String
	}
	if config.Password.Valid {
		esConfig.Password = config.Password.String
	}

	client, err := es.NewClient(esConfig)
	if err != nil {
		return nil, err
	}

	bulkIndexer, err := esutil.NewBulkIndexer(esutil.BulkIndexerConfig{
		Index:  "k6-metrics",
		Client: client,
	})
	if err != nil {
		params.Logger.Fatalf("Error creating the indexer: %s", err)
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
	res, err := o.client.Indices.Create("k6-metrics", o.client.Indices.Create.WithBody(bytes.NewReader(mapping)))
	if err != nil {
		return err
	}
	res.Body.Close()

	if periodicFlusher, err := output.NewPeriodicFlusher(time.Duration(o.config.FlushPeriod.Duration), o.flush); err != nil {
		return err
	} else {
		o.periodicFlusher = periodicFlusher
	}
	o.logger.Debug("Elasticsearch: starting writing")

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
		o.logger.Printf("ERROR: %s", err)
	} else {
		o.logger.Printf("ERROR: %s: %s", res.Error.Type, res.Error.Reason)
	}
}

func (o *Output) flush() {
	samplesContainers := o.GetBufferedSamples()
	for _, samplesContainer := range samplesContainers {
		samples := samplesContainer.GetSamples()

		for _, sample := range samples {
			for _, entry := range sample.GetSamples() {
				mappedEntry := elasticMetricEntry{
					MetricName:  entry.Metric.Name,
					MetricType:  entry.Metric.Type.String(),
					MetricValue: entry.Value,
					MetricTags:  entry.GetTags().Map(),
					SampleTags:  sample.GetTags().Map(),
					Time:        sample.Time,
				}
				data, err := json.Marshal(mappedEntry)
				if err != nil {
					o.logger.Fatalf("Cannot encode document: %s, %s", err, mappedEntry)
				}
				var item = esutil.BulkIndexerItem{
					Action:    "index",
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
}
