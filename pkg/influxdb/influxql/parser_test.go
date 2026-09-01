package influxql

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-influxdb-datasource/pkg/influxdb/influxql/buffered"
	"github.com/grafana/grafana-influxdb-datasource/pkg/influxdb/influxql/querydata"
	"github.com/grafana/grafana-influxdb-datasource/pkg/influxdb/models"
)

const simpleSeriesJSON = `{"results":[{"statement_id":0,"series":[{"name":"cpu","columns":["time","mean"],"values":[[1625097600000,42.5]]}]}]}`

// closeSpy records whether the response body was closed by parseResponse.
type closeSpy struct {
	io.Reader
	closed bool
}

func (c *closeSpy) Close() error {
	c.closed = true
	return nil
}

func httpResponse(body string, statusCode int, header http.Header) *http.Response {
	if header == nil {
		header = http.Header{}
	}
	return &http.Response{
		StatusCode: statusCode,
		Header:     header,
		Body:       &closeSpy{Reader: strings.NewReader(body)},
	}
}

// parserStrategies is every parser the executor can be constructed with.
// parseResponse must behave identically at the seam regardless of strategy.
var parserStrategies = map[string]responseParser{
	"buffered":  buffered.ResponseParse,
	"streaming": querydata.ResponseParse,
}

func TestParseResponseStrategies(t *testing.T) {
	for name, parse := range parserStrategies {
		t.Run(name, func(t *testing.T) {
			t.Run("parses a series response", func(t *testing.T) {
				e := &Executor{parse: parse}
				res := httpResponse(simpleSeriesJSON, http.StatusOK, nil)

				dr := e.parseResponse(context.Background(), res, &models.Query{RefID: "A"})

				require.NoError(t, dr.Error)
				require.Len(t, dr.Frames, 1)
			})

			t.Run("maps an influxdb error body", func(t *testing.T) {
				e := &Executor{parse: parse}
				res := httpResponse(`{"error":"bad thing"}`, http.StatusBadRequest, nil)

				dr := e.parseResponse(context.Background(), res, &models.Query{RefID: "A"})

				require.Error(t, dr.Error)
				require.Contains(t, dr.Error.Error(), "bad thing")
			})

			t.Run("closes the response body", func(t *testing.T) {
				e := &Executor{parse: parse}
				res := httpResponse(simpleSeriesJSON, http.StatusOK, nil)

				e.parseResponse(context.Background(), res, &models.Query{RefID: "A"})

				spy, ok := res.Body.(*closeSpy)
				require.True(t, ok)
				require.True(t, spy.closed)
			})
		})
	}
}

func TestParseResponseStampsCustomMetadata(t *testing.T) {
	header := http.Header{}
	header.Set("x-grafana-meta-add-region", "eu-west")
	e := &Executor{parse: buffered.ResponseParse}
	res := httpResponse(simpleSeriesJSON, http.StatusOK, header)

	dr := e.parseResponse(context.Background(), res, &models.Query{RefID: "A"})

	require.NoError(t, dr.Error)
	require.Len(t, dr.Frames, 1)
	require.Equal(t, map[string]any{"region": "eu-west"}, dr.Frames[0].Meta.Custom)
}
