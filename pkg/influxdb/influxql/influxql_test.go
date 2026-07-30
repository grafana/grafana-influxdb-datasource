package influxql

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
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

func TestQuery_httpConnectionFailureIsDownstream(t *testing.T) {
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

	req := &backend.QueryDataRequest{
		Queries: []backend.DataQuery{
			{RefID: "A", JSON: queryJSON},
		},
	}

	res, err := Query(context.Background(), nil, dsInfo, req)
	require.NoError(t, err)
	require.NotNil(t, res)

	dr, ok := res.Responses["A"]
	require.True(t, ok, "expected a response for RefID A")
	require.Error(t, dr.Error)
	assert.ErrorContains(t, dr.Error, connErr.Error())
	assert.Equal(t, backend.ErrorSourceDownstream, dr.ErrorSource)
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
