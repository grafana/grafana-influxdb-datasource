package influxdb

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/config"
	"github.com/stretchr/testify/require"
)

// Key read by config.GrafanaCfg.FeatureToggles(). The SDK only exports it
// from the experimental featuretoggles package, so it is repeated here.
const enabledFeaturesKey = "GF_INSTANCE_FEATURE_TOGGLES_ENABLE"

func parallelContext(limit int) context.Context {
	return config.WithGrafanaConfig(context.Background(), config.NewGrafanaCfg(map[string]string{
		enabledFeaturesKey:          "influxdbRunQueriesInParallel",
		config.ConcurrentQueryCount: fmt.Sprint(limit),
	}))
}

func makeQueries(n int) []backend.DataQuery {
	queries := make([]backend.DataQuery, 0, n)
	for i := 0; i < n; i++ {
		queries = append(queries, backend.DataQuery{RefID: fmt.Sprintf("Q%d", i)})
	}
	return queries
}

// inFlightTracker records the maximum number of concurrent execute calls.
type inFlightTracker struct {
	inFlight atomic.Int64
	max      atomic.Int64
}

func (tr *inFlightTracker) execute(ctx context.Context, q backend.DataQuery) backend.DataResponse {
	n := tr.inFlight.Add(1)
	defer tr.inFlight.Add(-1)
	for {
		old := tr.max.Load()
		if n <= old || tr.max.CompareAndSwap(old, n) {
			break
		}
	}
	time.Sleep(20 * time.Millisecond)
	return backend.DataResponse{}
}

func TestRunQueriesConcurrency(t *testing.T) {
	tests := []struct {
		name           string
		ctx            context.Context
		queries        int
		minMaxInFlight int64
		maxMaxInFlight int64
	}{
		{
			name:           "serial by default",
			ctx:            context.Background(),
			queries:        6,
			minMaxInFlight: 1,
			maxMaxInFlight: 1,
		},
		{
			name:           "parallel when toggle enabled",
			ctx:            parallelContext(4),
			queries:        8,
			minMaxInFlight: 2,
			maxMaxInFlight: 4,
		},
		{
			name: "invalid concurrency count falls back to default",
			ctx: config.WithGrafanaConfig(context.Background(), config.NewGrafanaCfg(map[string]string{
				enabledFeaturesKey:          "influxdbRunQueriesInParallel",
				config.ConcurrentQueryCount: "not-a-number",
			})),
			queries:        8,
			minMaxInFlight: 2,
			maxMaxInFlight: 10,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tracker := &inFlightTracker{}
			req := &backend.QueryDataRequest{Queries: makeQueries(tc.queries)}

			resp := runQueries(tc.ctx, req, tracker.execute)

			require.Len(t, resp.Responses, tc.queries)
			require.GreaterOrEqual(t, tracker.max.Load(), tc.minMaxInFlight)
			require.LessOrEqual(t, tracker.max.Load(), tc.maxMaxInFlight)
		})
	}
}

func TestRunQueriesKeysResponsesByRefID(t *testing.T) {
	req := &backend.QueryDataRequest{Queries: makeQueries(5)}

	resp := runQueries(parallelContext(3), req, func(ctx context.Context, q backend.DataQuery) backend.DataResponse {
		return backend.DataResponse{}
	})

	for i := 0; i < 5; i++ {
		_, ok := resp.Responses[fmt.Sprintf("Q%d", i)]
		require.True(t, ok, "missing response for Q%d", i)
	}
}

func TestRunQueriesIsolatesErrors(t *testing.T) {
	req := &backend.QueryDataRequest{Queries: makeQueries(4)}
	boom := errors.New("query exploded")

	resp := runQueries(parallelContext(4), req, func(ctx context.Context, q backend.DataQuery) backend.DataResponse {
		if q.RefID == "Q2" {
			return backend.DataResponse{Error: boom}
		}
		return backend.DataResponse{}
	})

	require.Len(t, resp.Responses, 4)
	require.ErrorIs(t, resp.Responses["Q2"].Error, boom)
	require.NoError(t, resp.Responses["Q0"].Error)
	require.NoError(t, resp.Responses["Q1"].Error)
	require.NoError(t, resp.Responses["Q3"].Error)
}

func TestRunQueriesCancelledContextReturns(t *testing.T) {
	ctx, cancel := context.WithCancelCause(parallelContext(2))
	cancel(errors.New("test cancels the request before execution"))

	var calls atomic.Int64
	req := &backend.QueryDataRequest{Queries: makeQueries(8)}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = runQueries(ctx, req, func(ctx context.Context, q backend.DataQuery) backend.DataResponse {
			calls.Add(1)
			return backend.DataResponse{}
		})
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("runQueries did not return after context cancellation")
	}
	_ = calls.Load() // the count is unasserted: ForEachJob may run 0..n jobs after cancellation
}
