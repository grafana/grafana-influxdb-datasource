package influxdb

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/httpclient"
	"github.com/stretchr/testify/require"
)

// contextWithForwardedHeader simulates what the SDK's headerMiddleware does:
// it injects a contextual HTTP client middleware that sets a header on outgoing
// requests — but only when ForwardHTTPHeaders is true on the HTTP client options.
func contextWithForwardedHeader(t *testing.T, key, value string) context.Context {
	t.Helper()
	return httpclient.WithContextualMiddleware(context.Background(),
		httpclient.MiddlewareFunc(func(opts httpclient.Options, next http.RoundTripper) http.RoundTripper {
			if !opts.ForwardHTTPHeaders {
				return next
			}
			return httpclient.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
				if req.Header.Get(key) == "" {
					req.Header.Set(key, value)
				}
				return next.RoundTrip(req)
			})
		}),
	)
}

func TestNewDatasource_ForwardHTTPHeaders(t *testing.T) {
	t.Run("HTTP client forwards OAuth and other HTTP headers from request context", func(t *testing.T) {
		// When oauthPassThru is enabled, the SDK's headerMiddleware puts forwarded
		// headers (Authorization, X-Id-Token, cookies) into the context as a
		// contextual HTTP client middleware. That middleware only fires if the HTTP
		// client was created with ForwardHTTPHeaders: true.

		var receivedAuthHeader string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedAuthHeader = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []any{
					map[string]any{
						"statement_id": 0,
						"series": []any{
							map[string]any{
								"name":    "cpu",
								"columns": []string{"time", "value"},
								"values":  [][]any{},
							},
						},
					},
				},
			})
		}))
		defer server.Close()

		dsSettings := backend.DataSourceInstanceSettings{
			URL: server.URL,
			JSONData: json.RawMessage(`{
				"version": "InfluxQL",
				"httpMode": "GET",
				"oauthPassThru": true
			}`),
		}

		instance, err := NewDatasource(context.Background(), dsSettings)
		require.NoError(t, err)
		ds := instance.(*DataSource)

		// Simulate the SDK's headerMiddleware: it reads OAuth headers from
		// req.GetHTTPHeaders() and injects them into the context via
		// httpclient.WithContextualMiddleware — but only if the HTTP client
		// opts have ForwardHTTPHeaders: true.
		//
		// We replicate that by injecting a contextual middleware directly,
		// which is exactly what the SDK does at runtime.
		oauthToken := "Bearer test-oauth-token-12345"
		ctx := contextWithForwardedHeader(t, "Authorization", oauthToken)

		query := backend.QueryDataRequest{
			Queries: []backend.DataQuery{
				{
					RefID: "A",
					JSON:  json.RawMessage(`{"rawQuery": true, "query": "SELECT * FROM cpu"}`),
				},
			},
		}

		_, err = ds.QueryData(ctx, &query)
		require.NoError(t, err)
		require.Equal(t, oauthToken, receivedAuthHeader,
			"OAuth token must be forwarded to InfluxDB when oauthPassThru is enabled")
	})
}
