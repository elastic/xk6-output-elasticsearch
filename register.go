package esoutput

import (
	"github.com/elastic/xk6-output-elasticsearch/pkg/esoutput"
	"go.k6.io/k6/output"
)

func init() {
	output.RegisterExtension("output-elasticsearch", func(p output.Params) (output.Output, error) {
		return esoutput.New(p)
	})
}
