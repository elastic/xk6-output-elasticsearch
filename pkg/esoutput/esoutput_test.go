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
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"go.k6.io/k6/v2/metrics"
	"go.k6.io/k6/v2/output"
)

func TestOutputWritesK6V2SamplesUsingElasticsearchV9BulkAPI(t *testing.T) {
	t.Parallel()

	bulkBodies := make(chan string, 1)
	server := newElasticsearchTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/":
			writeElasticsearchResponse(w, http.StatusOK, `{"version":{"number":"9.4.2"}}`)
		case r.Method == http.MethodPut && r.URL.Path == "/k6-metrics":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read create-index request: %v", err)
			}
			if !strings.Contains(string(body), `"Value"`) {
				t.Errorf("create-index mapping does not contain the Value field: %s", body)
			}
			writeElasticsearchResponse(w, http.StatusOK, `{"acknowledged":true}`)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/_bulk"):
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read bulk request: %v", err)
			}
			bulkBodies <- string(body)
			writeElasticsearchResponse(w, http.StatusOK, `{"errors":false,"items":[{"create":{"_index":"k6-metrics","status":201}}]}`)
		default:
			t.Errorf("unexpected Elasticsearch request: %s %s", r.Method, r.URL.RequestURI())
			writeElasticsearchResponse(w, http.StatusNotFound, `{}`)
		}
	})
	defer server.Close()

	out := newTestOutput(t, server.URL)
	if err := out.Start(); err != nil {
		t.Fatalf("start output: %v", err)
	}

	registry := metrics.NewRegistry()
	metric := registry.MustNewMetric("http_req_duration", metrics.Trend, metrics.Time)
	sampleTime := time.Date(2026, time.August, 6, 1, 2, 3, 0, time.UTC)
	out.AddMetricSamples([]metrics.SampleContainer{metrics.Sample{
		TimeSeries: metrics.TimeSeries{
			Metric: metric,
			Tags:   registry.RootTagSet().With("method", "GET"),
		},
		Time:  sampleTime,
		Value: 42.5,
	}})

	if err := out.Stop(); err != nil {
		t.Fatalf("stop output: %v", err)
	}

	var bulkBody string
	select {
	case bulkBody = <-bulkBodies:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the Elasticsearch bulk request")
	}
	lines := strings.Split(strings.TrimSpace(bulkBody), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected one bulk action and one document, got %d lines: %q", len(lines), bulkBody)
	}
	if !strings.Contains(lines[0], `"create"`) {
		t.Fatalf("expected a bulk create action, got %s", lines[0])
	}

	var document elasticMetricEntry
	if err := json.Unmarshal([]byte(lines[1]), &document); err != nil {
		t.Fatalf("decode bulk document: %v", err)
	}
	if document.MetricName != metric.Name || document.MetricType != metric.Type.String() {
		t.Errorf("unexpected metric identity: %#v", document)
	}
	if document.Value != 42.5 || !document.Time.Equal(sampleTime) {
		t.Errorf("unexpected metric value or time: %#v", document)
	}
	if document.Tags["method"] != "GET" {
		t.Errorf("unexpected metric tags: %#v", document.Tags)
	}
}

func TestOutputAcceptsExistingIndexResponse(t *testing.T) {
	t.Parallel()

	server := newElasticsearchTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/":
			writeElasticsearchResponse(w, http.StatusOK, `{}`)
		case r.Method == http.MethodPut && r.URL.Path == "/k6-metrics":
			writeElasticsearchResponse(w, http.StatusBadRequest, `{"error":{"type":"resource_already_exists_exception"},"status":400}`)
		default:
			t.Errorf("unexpected Elasticsearch request: %s %s", r.Method, r.URL.RequestURI())
			writeElasticsearchResponse(w, http.StatusNotFound, `{}`)
		}
	})
	defer server.Close()

	out := newTestOutput(t, server.URL)
	if err := out.Start(); err != nil {
		t.Fatalf("existing index should not prevent output startup: %v", err)
	}
	if err := out.Stop(); err != nil {
		t.Fatalf("stop output: %v", err)
	}
}

func TestOutputRejectsOtherBadRequestWhenCreatingIndex(t *testing.T) {
	t.Parallel()

	server := newElasticsearchTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/":
			writeElasticsearchResponse(w, http.StatusOK, `{}`)
		case r.Method == http.MethodPut && r.URL.Path == "/k6-metrics":
			writeElasticsearchResponse(w, http.StatusBadRequest, `{"error":{"type":"mapper_parsing_exception"},"status":400}`)
		default:
			t.Errorf("unexpected Elasticsearch request: %s %s", r.Method, r.URL.RequestURI())
			writeElasticsearchResponse(w, http.StatusNotFound, `{}`)
		}
	})
	defer server.Close()

	out := newTestOutput(t, server.URL)
	err := out.Start()
	if err == nil || !strings.Contains(err.Error(), "mapper_parsing_exception") {
		t.Fatalf("expected mapping error from output startup, got %v", err)
	}

	// Start failed before the periodic flusher was created, but the indexer and
	// client still need to be released in this direct unit-test invocation.
	if err := out.Stop(); err != nil {
		t.Fatalf("stop output after failed start: %v", err)
	}
}

func TestOutputUsesPrivilegeFallbackForSecuredCluster(t *testing.T) {
	t.Parallel()

	privilegeChecks := make(chan struct{}, 1)
	server := newElasticsearchTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/":
			writeElasticsearchResponse(w, http.StatusForbidden, `{"error":{"type":"security_exception"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/_security/user/_has_privileges":
			privilegeChecks <- struct{}{}
			writeElasticsearchResponse(w, http.StatusOK, `{"has_all_requested":true}`)
		default:
			t.Errorf("unexpected Elasticsearch request: %s %s", r.Method, r.URL.RequestURI())
			writeElasticsearchResponse(w, http.StatusNotFound, `{}`)
		}
	})
	defer server.Close()

	out := newTestOutput(t, server.URL)
	select {
	case <-privilegeChecks:
	default:
		t.Fatal("expected the secured-cluster privilege fallback request")
	}
	if err := out.Stop(); err != nil {
		t.Fatalf("stop output: %v", err)
	}
}

func newTestOutput(t *testing.T, elasticsearchURL string) *Output {
	t.Helper()

	logger := logrus.New()
	logger.SetOutput(io.Discard)
	created, err := New(output.Params{
		Logger: logger,
		Environment: map[string]string{
			"K6_ELASTICSEARCH_URL":          elasticsearchURL,
			"K6_ELASTICSEARCH_FLUSH_PERIOD": "1h",
		},
	})
	if err != nil {
		t.Fatalf("create output: %v", err)
	}

	out, ok := created.(*Output)
	if !ok {
		t.Fatalf("unexpected output type %T", created)
	}
	return out
}

func newElasticsearchTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(handler)
}

func writeElasticsearchResponse(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Elastic-Product", "Elasticsearch")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}
