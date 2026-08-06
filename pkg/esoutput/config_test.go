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
 */

package esoutput

import (
	"encoding/json"
	"testing"
	"time"
)

func TestGetConsolidatedConfigPrecedence(t *testing.T) {
	t.Parallel()

	config, err := GetConsolidatedConfig(
		json.RawMessage(`{"url":"http://json:9200","indexName":"json-index","flushPeriod":"2s"}`),
		map[string]string{
			"K6_ELASTICSEARCH_URL":          "http://env:9200",
			"K6_ELASTICSEARCH_INDEX_NAME":   "env-index",
			"K6_ELASTICSEARCH_FLUSH_PERIOD": "3s",
		},
		"url=http://argument:9200,indexName=argument-index,flushPeriod=4s",
	)
	if err != nil {
		t.Fatalf("get consolidated config: %v", err)
	}

	if config.Url.String != "http://argument:9200" {
		t.Errorf("unexpected URL: %s", config.Url.String)
	}
	if config.IndexName.String != "argument-index" {
		t.Errorf("unexpected index name: %s", config.IndexName.String)
	}
	if time.Duration(config.FlushPeriod.Duration) != 4*time.Second {
		t.Errorf("unexpected flush period: %s", config.FlushPeriod.Duration)
	}
}

func TestGetConsolidatedConfigDefaults(t *testing.T) {
	t.Parallel()

	config, err := GetConsolidatedConfig(nil, nil, "")
	if err != nil {
		t.Fatalf("get default config: %v", err)
	}

	if config.Url.String != "http://localhost:9200" {
		t.Errorf("unexpected default URL: %s", config.Url.String)
	}
	if config.IndexName.String != defaultIndexName {
		t.Errorf("unexpected default index: %s", config.IndexName.String)
	}
	if time.Duration(config.FlushPeriod.Duration) != defaultFlushPeriod {
		t.Errorf("unexpected default flush period: %s", config.FlushPeriod.Duration)
	}
}
