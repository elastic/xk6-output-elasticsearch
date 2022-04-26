# xk6-output-elasticsearch
k6 extension for publishing test-run metrics to Elasticsearch.

### Usage

To build k6 binary with the Elasticsearch output extension use:
```
# Install xk6
go install go.k6.io/xk6/cmd/xk6@latest

# Build the k6 binary
xk6 build --with github.com/elastic/xk6-output-elasticsearch@latest 
```

You can run new k6 binary against a Cloud cluster with:
```
export K6_ELASTICSEARCH_CLOUD_ID=your-cloud-id-here
export K6_ELASTICSEARCH_USER=elastic
export K6_ELASTICSEARCH_PASSWORD=your-password-here

./k6 run script.js -o output-elasticsearch
```

Alternatively you can run against a local cluster (without security, self-signed certificates are not supported yet):

```
export K6_ELASTICSEARCH_URL=http://localhost:9200

./k6 run script.js -o output-elasticsearch
```

### License
 
This software is licensed under the Apache License, version 2 ("ALv2"), quoted below.

Copyright 2022 Elasticsearch <https://www.elastic.co>

Licensed under the Apache License, Version 2.0 (the "License"); you may not
use this file except in compliance with the License. You may obtain a copy of
the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
License for the specific language governing permissions and limitations under
the License.