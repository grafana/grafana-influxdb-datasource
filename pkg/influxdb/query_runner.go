package influxdb

import (
	"context"
	"sync"

	"github.com/grafana/dskit/concurrency"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/config"
)

const (
	runQueriesInParallelToggle  = "influxdbRunQueriesInParallel"
	defaultConcurrentQueryCount = 10
)

// runQueries executes every query in req through execute and assembles the
// responses keyed by RefID. When the influxdbRunQueriesInParallel feature
// toggle is enabled queries run concurrently, bounded by the Grafana
// concurrent query count; otherwise they run serially. execute must be safe
// for concurrent use and must report failures inside the returned
// DataResponse rather than panicking, so one failing query never affects its
// siblings.
func runQueries(ctx context.Context, req *backend.QueryDataRequest, execute func(context.Context, backend.DataQuery) backend.DataResponse) *backend.QueryDataResponse {
	response := backend.NewQueryDataResponse()
	cfg := config.GrafanaConfigFromContext(ctx)

	if !cfg.FeatureToggles().IsEnabled(runQueriesInParallelToggle) {
		for _, q := range req.Queries {
			response.Responses[q.RefID] = execute(ctx, q)
		}
		return response
	}

	concurrentQueryCount, err := cfg.ConcurrentQueryCount()
	if err != nil {
		logger.FromContext(ctx).Debug("Concurrent query count read/parse error, using default", "err", err, "default", defaultConcurrentQueryCount)
		concurrentQueryCount = defaultConcurrentQueryCount
	}

	var responseLock sync.Mutex
	err = concurrency.ForEachJob(ctx, len(req.Queries), concurrentQueryCount, func(ctx context.Context, idx int) error {
		q := req.Queries[idx]
		res := execute(ctx, q)
		responseLock.Lock()
		defer responseLock.Unlock()
		response.Responses[q.RefID] = res
		// Errors are reported per query inside res; returning nil stops the
		// errgroup from cancelling the remaining queries.
		return nil
	})
	if err != nil {
		logger.FromContext(ctx).Debug("Influxdb concurrent query error", "err", err)
	}
	return response
}
