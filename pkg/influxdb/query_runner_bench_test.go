package influxdb

import (
	"context"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

// benchmarkExecute simulates a query round-trip with fixed latency so the
// benchmark isolates fan-out overhead and overlap.
func benchmarkExecute(ctx context.Context, q backend.DataQuery) backend.DataResponse {
	time.Sleep(5 * time.Millisecond)
	return backend.DataResponse{}
}

func BenchmarkRunQueries(b *testing.B) {
	req := &backend.QueryDataRequest{Queries: makeQueries(10)}

	b.Run("serial", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			runQueries(context.Background(), req, benchmarkExecute)
		}
	})

	b.Run("parallel", func(b *testing.B) {
		ctx := parallelContext(10)
		for i := 0; i < b.N; i++ {
			runQueries(ctx, req, benchmarkExecute)
		}
	})
}
