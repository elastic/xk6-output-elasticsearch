# xk6-output-elasticsearch
k6 extension for publishing test-run metrics to Elasticsearch.

### Prerequisites

Go 1.17 or better (verify with `go version`).

### Getting started

```
git clone git@github.com:elastic/xk6-output-elasticsearch.git
# build k6 with the Elasticsearch output extension
make
```

You can run the new k6 binary against a Cloud cluster with:
```
export K6_ELASTICSEARCH_CLOUD_ID=your-cloud-id-here
export K6_ELASTICSEARCH_USER=elastic
export K6_ELASTICSEARCH_PASSWORD=your-password-here

./k6 run script.js -o output-elasticsearch
```

Alternatively you can send metrics to a local cluster (without security, self-signed certificates are not supported yet):

```
export K6_ELASTICSEARCH_URL=http://localhost:9200

./k6 run script.js -o output-elasticsearch
```

The metrics are stored in the index `k6-metrics` which will be automatically created by this extension. See the [mapping](pkg/esoutput/mapping.json) for details.
