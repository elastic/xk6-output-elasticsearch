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
	"encoding/json"
	"strconv"
	"time"

	"github.com/guregu/null/v5"

	"github.com/kubernetes/helm/pkg/strvals"
	"go.k6.io/k6/lib/types"
)

const (
	defaultFlushPeriod = time.Second
	defaultIndexName   = "k6-metrics"
)

type Config struct {
	Url                null.String `json:"url" envconfig:"K6_ELASTICSEARCH_URL"`
	CloudID            null.String `json:"cloud-id"  envconfig:"K6_ELASTICSEARCH_CLOUD_ID"`
	CACert             null.String `json:"caCertFile" envconfig:"K6_ELASTICSEARCH_CA_CERT_FILE"`
	InsecureSkipVerify null.Bool   `json:"insecureSkipVerify" envconfig:"K6_ELASTICSEARCH_INSECURE_SKIP_VERIFY"`

	ClientCert null.String `json:"clientCertFile" envconfig:"K6_ELASTICSEARCH_CLIENT_CERT_FILE"`
	ClientKey  null.String `json:"clientKeyFile" envconfig:"K6_ELASTICSEARCH_CLIENT_KEY_FILE"`

	User                null.String `json:"user" envconfig:"K6_ELASTICSEARCH_USER"`
	Password            null.String `json:"password" envconfig:"K6_ELASTICSEARCH_PASSWORD"`
	APIKey              null.String `json:"apiKey" envconfig:"K6_ELASTICSEARCH_API_KEY"`
	ServiceAccountToken null.String `json:"serviceAccountToken" envconfig:"K6_ELASTICSEARCH_SERVICE_ACCOUNT_TOKEN"`

	FlushPeriod types.NullDuration `json:"flushPeriod" envconfig:"K6_ELASTICSEARCH_FLUSH_PERIOD"`
	IndexName   null.String        `json:"indexName" envconfig:"K6_ELASTICSEARCH_INDEX_NAME"`
}

func NewConfig() Config {
	return Config{
		Url:                 null.StringFrom("http://localhost:9200"),
		CloudID:             null.NewString("", false),
		APIKey:              null.NewString("", false),
		CACert:              null.NewString("", false),
		InsecureSkipVerify:  null.BoolFrom(false),
		User:                null.NewString("", false),
		Password:            null.NewString("", false),
		ServiceAccountToken: null.NewString("", false),
		FlushPeriod:         types.NullDurationFrom(defaultFlushPeriod),
		IndexName:           null.StringFrom(defaultIndexName),
	}
}

// From here till the end of the file partial duplicates waiting for config refactor (k6 #883)

func (base Config) Apply(applied Config) Config {
	if applied.Url.Valid {
		base.Url = applied.Url
	}
	if applied.CloudID.Valid {
		base.CloudID = applied.CloudID
	}
	if applied.APIKey.Valid {
		base.APIKey = applied.APIKey
	}

	if applied.CACert.Valid {
		base.CACert = applied.CACert
	}
	if applied.InsecureSkipVerify.Valid {
		base.InsecureSkipVerify = applied.InsecureSkipVerify
	}

	if applied.ClientCert.Valid {
		base.ClientCert = applied.ClientCert
	}
	if applied.ClientKey.Valid {
		base.ClientKey = applied.ClientKey
	}

	if applied.User.Valid {
		base.User = applied.User
	}

	if applied.Password.Valid {
		base.Password = applied.Password
	}

	if applied.ServiceAccountToken.Valid {
		base.ServiceAccountToken = applied.ServiceAccountToken
	}

	if applied.FlushPeriod.Valid {
		base.FlushPeriod = applied.FlushPeriod
	}
	if applied.IndexName.Valid {
		base.IndexName = applied.IndexName
	}

	return base
}

// ParseArg takes an arg string and converts it to a config
func ParseArg(arg string) (Config, error) {
	var c Config
	params, err := strvals.Parse(arg)
	if err != nil {
		return c, err
	}

	if v, ok := params["url"].(string); ok {
		c.Url = null.StringFrom(v)
	}

	if v, ok := params["cloud-id"].(string); ok {
		c.CloudID = null.StringFrom(v)
	}

	if v, ok := params["caCertFile"].(string); ok {
		c.CACert = null.StringFrom(v)
	}

	if v, ok := params["insecureSkipVerify"].(bool); ok {
		c.InsecureSkipVerify = null.BoolFrom(v)
	}

	if v, ok := params["clientCertFile"].(string); ok {
		c.ClientCert = null.StringFrom(v)
	}
	if v, ok := params["clientKeyFile"].(string); ok {
		c.ClientKey = null.StringFrom(v)
	}

	if v, ok := params["user"].(string); ok {
		c.User = null.StringFrom(v)
	}

	if v, ok := params["password"].(string); ok {
		c.Password = null.StringFrom(v)
	}
	if v, ok := params["apiKey"].(string); ok {
		c.APIKey = null.StringFrom(v)
	}
	if v, ok := params["serviceAccountToken"].(string); ok {
		c.ServiceAccountToken = null.StringFrom(v)
	}

	if v, ok := params["flushPeriod"].(string); ok {
		if err := c.FlushPeriod.UnmarshalText([]byte(v)); err != nil {
			return c, err
		}
	}
	if v, ok := params["indexName"].(string); ok {
		c.IndexName = null.StringFrom(v)
	}

	return c, nil
}

// GetConsolidatedConfig combines {default config values + JSON config +
// environment vars + arg config values}, and returns the final result.
func GetConsolidatedConfig(jsonRawConf json.RawMessage, env map[string]string, arg string) (Config, error) {
	result := NewConfig()
	if jsonRawConf != nil {
		jsonConf := Config{}
		if err := json.Unmarshal(jsonRawConf, &jsonConf); err != nil {
			return result, err
		}
		result = result.Apply(jsonConf)
	}

	getEnvBool := func(env map[string]string, name string) (null.Bool, error) {
		if v, vDefined := env[name]; vDefined {
			if b, err := strconv.ParseBool(v); err != nil {
				return null.NewBool(false, false), err
			} else {
				return null.BoolFrom(b), nil
			}
		}
		return null.NewBool(false, false), nil
	}

	// envconfig is not processing some undefined vars (at least duration) so apply them manually
	if flushPeriod, flushPeriodDefined := env["K6_ELASTICSEARCH_FLUSH_PERIOD"]; flushPeriodDefined {
		if err := result.FlushPeriod.UnmarshalText([]byte(flushPeriod)); err != nil {
			return result, err
		}
	}

	if url, defined := env["K6_ELASTICSEARCH_URL"]; defined {
		result.Url = null.StringFrom(url)
	}

	if cloudId, defined := env["K6_ELASTICSEARCH_CLOUD_ID"]; defined {
		result.CloudID = null.StringFrom(cloudId)
	}

	if ca, defined := env["K6_ELASTICSEARCH_CA_CERT_FILE"]; defined {
		result.CACert = null.StringFrom(ca)
	}

	if skipVerify, err := getEnvBool(env, "K6_ELASTICSEARCH_INSECURE_SKIP_VERIFY"); err != nil {
		return result, err
	} else {
		if skipVerify.Valid {
			result.InsecureSkipVerify = skipVerify
		}
	}

	if clientCert, defined := env["K6_ELASTICSEARCH_CLIENT_CERT_FILE"]; defined {
		result.ClientCert = null.StringFrom(clientCert)
	}
	if clientKey, defined := env["K6_ELASTICSEARCH_CLIENT_KEY_FILE"]; defined {
		result.ClientKey = null.StringFrom(clientKey)
	}

	if user, defined := env["K6_ELASTICSEARCH_USER"]; defined {
		result.User = null.StringFrom(user)
	}

	if password, defined := env["K6_ELASTICSEARCH_PASSWORD"]; defined {
		result.Password = null.StringFrom(password)
	}
	if apiKey, defined := env["K6_ELASTICSEARCH_API_KEY"]; defined {
		result.APIKey = null.StringFrom(apiKey)
	}
	if serviceAccountToken, defined := env["K6_ELASTICSEARCH_SERVICE_ACCOUNT_TOKEN"]; defined {
		result.ServiceAccountToken = null.StringFrom(serviceAccountToken)
	}
	if indexName, defined := env["K6_ELASTICSEARCH_INDEX_NAME"]; defined {
		result.IndexName = null.StringFrom(indexName)
	}

	if arg != "" {
		argConf, err := ParseArg(arg)
		if err != nil {
			return result, err
		}

		result = result.Apply(argConf)
	}

	return result, nil
}
