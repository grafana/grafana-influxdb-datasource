package influxql

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-plugin-sdk-go/backend"

	"github.com/grafana/grafana-influxdb-datasource/pkg/influxdb/models"
)

// roundTripperFunc lets us stub an *http.Client transport in tests.
type roundTripperFunc func(req *http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestExecutorExecute_httpConnectionFailureIsDownstream(t *testing.T) {
	// Simulate a transport-level failure (e.g. a TLS handshake error) from
	// the request to the InfluxDB /query endpoint.
	connErr := errors.New("remote error: tls: internal error")
	dsInfo := &models.DatasourceInfo{
		URL:      "http://influxdb:1337",
		DbName:   "testdb",
		HTTPMode: "GET",
		HTTPClient: &http.Client{
			Transport: roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
				return nil, connErr
			}),
		},
	}

	queryJSON, err := json.Marshal(map[string]any{
		"query":    "SELECT * FROM testdb",
		"rawQuery": true,
	})
	require.NoError(t, err)

	executor, err := NewExecutor(context.Background(), nil, dsInfo)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, executor.Close()) })

	res := executor.Execute(context.Background(), backend.DataQuery{
		RefID: "A",
		JSON:  queryJSON,
	})

	require.Error(t, res.Error)
	assert.ErrorContains(t, res.Error, connErr.Error())
	assert.Equal(t, backend.ErrorSourceDownstream, res.ErrorSource)
}

type staticRoundTripper struct {
	body   string
	status int
}

func (s *staticRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: s.status,
		Body:       io.NopCloser(strings.NewReader(s.body)),
		Header:     http.Header{},
	}, nil
}

func TestExecutorExecute(t *testing.T) {
	body := `{"results":[{"statement_id":0,"series":[{"name":"cpu","columns":["time","value"],"values":[[1622505600000,42]]}]}]}`
	dsInfo := &models.DatasourceInfo{
		URL:        "http://influx.example",
		DbName:     "fixtures",
		HTTPMode:   "POST",
		HTTPClient: &http.Client{Transport: &staticRoundTripper{body: body, status: http.StatusOK}},
	}

	executor, err := NewExecutor(context.Background(), nil, dsInfo)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, executor.Close()) })

	res := executor.Execute(context.Background(), backend.DataQuery{
		RefID: "A",
		JSON:  []byte(`{"query": "SELECT value FROM cpu", "rawQuery": true}`),
	})

	require.NoError(t, res.Error)
	require.NotEmpty(t, res.Frames)
}

func TestExecutorExecuteBadQueryJSON(t *testing.T) {
	dsInfo := &models.DatasourceInfo{
		URL:        "http://influx.example",
		HTTPMode:   "POST",
		HTTPClient: &http.Client{Transport: &staticRoundTripper{body: "{}", status: http.StatusOK}},
	}
	executor, err := NewExecutor(context.Background(), nil, dsInfo)
	require.NoError(t, err)

	res := executor.Execute(context.Background(), backend.DataQuery{RefID: "A", JSON: []byte(`{invalid`)})

	require.Error(t, res.Error)
	require.Equal(t, backend.ErrorSourceDownstream, res.ErrorSource)
}

func TestExecutor_createRequest(t *testing.T) {
	logger := backend.NewLoggerWith("logger", "tsdb.influx_influxql_test")
	datasource := &models.DatasourceInfo{
		URL:      "http://awesome-influxdb:1337",
		DbName:   "awesome-db",
		HTTPMode: "GET",
	}
	query := "SELECT awesomeness FROM somewhere"

	t.Run("createRequest with GET httpMode", func(t *testing.T) {
		req, err := createRequest(context.Background(), logger, datasource, query, defaultRetentionPolicy)

		require.NoError(t, err)

		assert.Equal(t, "GET", req.Method)

		q := req.URL.Query().Get("q")
		assert.Equal(t, query, q)

		assert.Nil(t, req.Body)
	})

	t.Run("createRequest with POST httpMode", func(t *testing.T) {
		datasource.HTTPMode = "POST"
		req, err := createRequest(context.Background(), logger, datasource, query, defaultRetentionPolicy)
		require.NoError(t, err)

		assert.Equal(t, "POST", req.Method)

		q := req.URL.Query().Get("q")
		assert.Empty(t, q)

		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)

		testBodyValues := url.Values{}
		testBodyValues.Add("q", query)
		testBody := testBodyValues.Encode()
		assert.Equal(t, testBody, string(body))
	})

	t.Run("createRequest with PUT httpMode", func(t *testing.T) {
		datasource.HTTPMode = "PUT"
		_, err := createRequest(context.Background(), logger, datasource, query, defaultRetentionPolicy)
		require.EqualError(t, err, ErrInvalidHttpMode.Error())
	})
}

func TestReadCustomMetadata(t *testing.T) {
	t.Run("should read nothing if no X-Grafana-Meta-Add-<Thing> header exists", func(t *testing.T) {
		header := http.Header{}
		header.Add("content-type", "text/html")
		header.Add("content-encoding", "gzip")
		res := &http.Response{
			Header: header,
		}
		result := readCustomMetadata(res)
		require.Nil(t, result)
	})

	t.Run("should read X-Grafana-Meta-Add-<Thing> header", func(t *testing.T) {
		header := http.Header{}
		header.Add("content-type", "text/html")
		header.Add("content-encoding", "gzip")
		header.Add("X-Grafana-Meta-Add-TestThing", "test1234")
		res := &http.Response{
			Header: header,
		}
		result := readCustomMetadata(res)
		expected := map[string]any{
			"testthing": "test1234",
		}
		require.NotNil(t, result)
		require.Equal(t, expected, result)
	})

	t.Run("should read multiple X-Grafana-Meta-Add-<Thing> header", func(t *testing.T) {
		header := http.Header{}
		header.Add("content-type", "text/html")
		header.Add("content-encoding", "gzip")
		header.Add("X-Grafana-Meta-Add-TestThing", "test111")
		header.Add("X-Grafana-Meta-Add-TestThing2", "test222")
		header.Add("X-Grafana-Meta-Add-Test-Other", "other")
		res := &http.Response{
			Header: header,
		}
		result := readCustomMetadata(res)
		expected := map[string]any{
			"testthing":  "test111",
			"testthing2": "test222",
			"test-other": "other",
		}
		require.NotNil(t, result)
		require.Equal(t, expected, result)
	})
}
