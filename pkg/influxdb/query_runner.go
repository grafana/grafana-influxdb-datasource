package influxdb

import (
	"context"
	"fmt"
	"sync"

	"github.com/grafana/dskit/concurrency"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/tracing"
	"github.com/grafana/grafana-plugin-sdk-go/config"

	"github.com/grafana/grafana-influxdb-datasource/pkg/influxdb/flux"
	"github.com/grafana/grafana-influxdb-datasource/pkg/influxdb/fsql"
	"github.com/grafana/grafana-influxdb-datasource/pkg/influxdb/influxql"
	"github.com/grafana/grafana-influxdb-datasource/pkg/influxdb/models"
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

// queryExecutor executes a single query. Implementations must be safe for
// concurrent use and report failures inside the returned response.
type queryExecutor interface {
	Execute(ctx context.Context, query backend.DataQuery) backend.DataResponse
	Close() error
}

// newQueryExecutor builds the executor for the datasource's query language.
func newQueryExecutor(ctx context.Context, dsInfo *models.DatasourceInfo) (queryExecutor, error) {
	switch dsInfo.Version {
	case influxVersionFlux:
		return flux.NewExecutor(dsInfo)
	case influxVersionInfluxQL:
		return influxql.NewExecutor(ctx, tracing.DefaultTracer(), dsInfo)
	case influxVersionSQL:
		return fsql.NewExecutor(dsInfo)
	default:
		return nil, fmt.Errorf("unknown influxdb version")
	}
}

// executeRequest runs every query in req through the language executor via
// the shared fan-out. It is the single execution path for QueryData and the
// health checks.
func executeRequest(ctx context.Context, dsInfo *models.DatasourceInfo, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	executor, err := newQueryExecutor(ctx, dsInfo)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := executor.Close(); err != nil {
			logger.FromContext(ctx).Warn("Failed to close query executor", "err", err)
		}
	}()
	return runQueries(ctx, req, executor.Execute), nil
}
