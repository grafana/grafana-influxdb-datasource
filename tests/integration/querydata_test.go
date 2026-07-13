package integration

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/stretchr/testify/require"
)

func TestIntegrationSerialAndParallelAgree(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	for _, inst := range instances(t) {
		t.Run(inst.name, func(t *testing.T) {
			requireReachable(t, inst.addr)
			ds := newDataSource(t, inst.settings)

			serial, err := ds.QueryData(serialContext(), &backend.QueryDataRequest{Queries: inst.queries})
			require.NoError(t, err)
			parallel, err := ds.QueryData(parallelContext(), &backend.QueryDataRequest{Queries: inst.queries})
			require.NoError(t, err)

			require.Len(t, serial.Responses, len(inst.queries))
			require.Len(t, parallel.Responses, len(inst.queries))

			for _, q := range inst.queries {
				sRes := serial.Responses[q.RefID]
				pRes := parallel.Responses[q.RefID]
				require.NoError(t, sRes.Error, "serial query %s failed", q.RefID)
				require.NoError(t, pRes.Error, "parallel query %s failed", q.RefID)
				require.NotEmpty(t, sRes.Frames, "serial query %s returned no frames", q.RefID)

				sJSON, err := json.Marshal(sRes.Frames)
				require.NoError(t, err)
				pJSON, err := json.Marshal(pRes.Frames)
				require.NoError(t, err)
				require.JSONEq(t, string(sJSON), string(pJSON), "serial and parallel disagree for %s", q.RefID)
			}
		})
	}
}

func TestIntegrationErrorIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	for _, inst := range instances(t) {
		t.Run(inst.name, func(t *testing.T) {
			requireReachable(t, inst.addr)
			ds := newDataSource(t, inst.settings)

			batch := append([]backend.DataQuery{inst.invalid}, inst.queries...)

			for _, mode := range []struct {
				name string
				ctx  func() context.Context
			}{
				{"serial", serialContext},
				{"parallel", parallelContext},
			} {
				t.Run(mode.name, func(t *testing.T) {
					resp, err := ds.QueryData(mode.ctx(), &backend.QueryDataRequest{Queries: batch})
					require.NoError(t, err)
					require.Len(t, resp.Responses, len(batch), "failing query must not abandon the batch")
					require.Error(t, resp.Responses["BAD"].Error)
					for _, q := range inst.queries {
						require.NoError(t, resp.Responses[q.RefID].Error, "query %s should succeed alongside a failing sibling", q.RefID)
					}
				})
			}
		})
	}
}

func TestIntegrationCancellationReturnsPromptly(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	for _, inst := range instances(t) {
		t.Run(inst.name, func(t *testing.T) {
			requireReachable(t, inst.addr)
			ds := newDataSource(t, inst.settings)

			ctx, cancel := context.WithCancelCause(parallelContext())
			cancel(errors.New("test cancels the request before execution"))

			done := make(chan struct{})
			go func() {
				defer close(done)
				// A cancelled request must return promptly; the responses
				// themselves are unspecified (typically per-query errors).
				_, _ = ds.QueryData(ctx, &backend.QueryDataRequest{Queries: inst.queries})
			}()

			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Fatal("QueryData did not return promptly after cancellation")
			}
		})
	}
}
