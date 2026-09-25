package influxdb

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/httpclient"
	"github.com/stretchr/testify/require"
)

const (
	reproAccessToken = "Bearer access-token"
	reproIDToken     = "id-token"
	reproUser        = "influx-user"
	reproPassword    = "influx-pass"
)

func TestHTTPClientAuth_oauthWinsOverInfluxQLUserPassword(t *testing.T) {
	captured := captureAuthHeaders(t)

	inst, err := NewDatasource(context.Background(), influxQLSettings(captured.URL))
	require.NoError(t, err)
	ds := inst.(*DataSource)

	ctx := withSDKStyleHeaderForwarding(context.Background(), reproAccessToken, reproIDToken)
	_, err = ds.QueryData(ctx, influxQLProbeRequest())
	require.NoError(t, err)

	require.Equal(t, reproAccessToken, captured.Authorization)
	require.Equal(t, reproIDToken, captured.IDToken)
}

func TestHTTPClientAuth_basicFallbackWithoutForwardedHeaders(t *testing.T) {
	captured := captureAuthHeaders(t)

	inst, err := NewDatasource(context.Background(), influxQLSettings(captured.URL))
	require.NoError(t, err)
	ds := inst.(*DataSource)

	_, err = ds.QueryData(context.Background(), influxQLProbeRequest())
	require.NoError(t, err)

	require.Equal(t, "Basic "+basic(reproUser, reproPassword), captured.Authorization)
	require.Empty(t, captured.IDToken)
}

func TestBasicAuthAfterForwardedHeaders_placesBasicAfterContextual(t *testing.T) {
	reordered := basicAuthAfterForwardedHeaders(httpclient.Options{}, httpclient.DefaultMiddlewares())
	names := make([]string, 0, len(reordered))
	for _, m := range reordered {
		names = append(names, middlewareName(m))
	}

	contextualIdx, basicIdx, customIdx := -1, -1, -1
	for i, name := range names {
		switch name {
		case httpclient.ContextualMiddlewareName:
			contextualIdx = i
		case httpclient.BasicAuthenticationMiddlewareName:
			basicIdx = i
		case httpclient.CustomHeadersMiddlewareName:
			customIdx = i
		}
	}
	require.Greater(t, contextualIdx, -1)
	require.Greater(t, basicIdx, -1)
	require.Greater(t, customIdx, -1)
	require.Less(t, customIdx, contextualIdx, "ContextualMiddleware stays after CustomHeaders")
	require.Equal(t, contextualIdx+1, basicIdx, "Basic Auth is the fallback after forwarded headers")
}

func TestBasicAuthAfterForwardedHeaders_keepsOriginalOrderWithoutContextual(t *testing.T) {
	var withoutContextual []httpclient.Middleware
	for _, m := range httpclient.DefaultMiddlewares() {
		if middlewareName(m) != httpclient.ContextualMiddlewareName {
			withoutContextual = append(withoutContextual, m)
		}
	}

	got := basicAuthAfterForwardedHeaders(httpclient.Options{}, withoutContextual)
	require.Equal(t, len(withoutContextual), len(got))
	for i := range got {
		require.Equal(t, middlewareName(withoutContextual[i]), middlewareName(got[i]))
	}
}

type capturedAuth struct {
	URL           string
	Authorization string
	IDToken       string
}

func captureAuthHeaders(t *testing.T) *capturedAuth {
	t.Helper()
	out := &capturedAuth{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		out.Authorization = r.Header.Get("Authorization")
		out.IDToken = r.Header.Get("X-Id-Token")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"results":[{"statement_id":0}]}`))
	}))
	t.Cleanup(srv.Close)
	out.URL = srv.URL
	return out
}

func influxQLSettings(url string) backend.DataSourceInstanceSettings {
	return backend.DataSourceInstanceSettings{
		URL:  url,
		User: reproUser,
		JSONData: []byte(`{
			"version": "InfluxQL",
			"dbName": "fixtures",
			"httpMode": "GET",
			"oauthPassThru": true
		}`),
		DecryptedSecureJSONData: map[string]string{
			"password": reproPassword,
		},
	}
}

func influxQLProbeRequest() *backend.QueryDataRequest {
	return &backend.QueryDataRequest{
		Queries: []backend.DataQuery{{
			RefID:     "A",
			JSON:      []byte(`{"query": "SHOW measurements", "rawQuery": true}`),
			QueryType: "health",
		}},
	}
}

func withSDKStyleHeaderForwarding(parent context.Context, authorization, idToken string) context.Context {
	return httpclient.WithContextualMiddleware(parent, httpclient.MiddlewareFunc(func(opts httpclient.Options, next http.RoundTripper) http.RoundTripper {
		if !opts.ForwardHTTPHeaders {
			return next
		}
		return httpclient.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			if req.Header.Get("Authorization") == "" && authorization != "" {
				req.Header.Set("Authorization", authorization)
			}
			if req.Header.Get("X-Id-Token") == "" && idToken != "" {
				req.Header.Set("X-Id-Token", idToken)
			}
			return next.RoundTrip(req)
		})
	}))
}

func basic(user, password string) string {
	return base64.StdEncoding.EncodeToString([]byte(user + ":" + password))
}
