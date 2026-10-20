package influxql

import (
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-influxdb-datasource/pkg/influxdb/influxql/buffered"
	"github.com/grafana/grafana-influxdb-datasource/pkg/influxdb/influxql/querydata"
	"github.com/grafana/grafana-influxdb-datasource/pkg/influxdb/models"
)

func TestParsersEmptyResults(t *testing.T) {
	parsers := map[string]func(io.ReadCloser, int, *models.Query) *backend.DataResponse{
		"buffered":  buffered.ResponseParse,
		"streaming": querydata.ResponseParse,
	}
	for name, parse := range parsers {
		for _, format := range []string{"time_series", "table"} {
			for _, status := range []int{200, 206, 299} {
				for _, body := range []string{`{}`, `{"results":[]}`, `{"results":null}`, `{"results":[{}]}`, `{"code":"empty"}`, `{"metadata":{}}`} {
					t.Run(fmt.Sprintf("%s/%s/%d/%s", name, format, status, body), func(t *testing.T) {
						var response *backend.DataResponse
						require.NotPanics(t, func() {
							response = parse(io.NopCloser(strings.NewReader(body)), status, &models.Query{ResultFormat: format})
						})
						require.NotNil(t, response)
						require.NoError(t, response.Error)
						require.Empty(t, response.Frames)
					})
				}
			}
		}
	}
}

func TestParsersMessageWithoutResults(t *testing.T) {
	parsers := map[string]func(io.ReadCloser, int, *models.Query) *backend.DataResponse{
		"buffered":  buffered.ResponseParse,
		"streaming": querydata.ResponseParse,
	}
	for name, parse := range parsers {
		for _, body := range []string{
			`{"code":"invalid","message":"query failed"}`,
			`{"message":"query failed","code":"invalid"}`,
			`{"results":[],"message":"query failed"}`,
			`{"results":null,"message":"query failed"}`,
		} {
			t.Run(name+"/"+body, func(t *testing.T) {
				var response *backend.DataResponse
				require.NotPanics(t, func() {
					response = parse(io.NopCloser(strings.NewReader(body)), 200, &models.Query{})
				})
				require.NotNil(t, response)
				require.EqualError(t, response.Error, "query failed")
				require.Equal(t, backend.ErrorSourceDownstream, response.ErrorSource)
				require.Empty(t, response.Frames)
			})
		}
	}
}

func TestParsersHTTPErrorWithoutResults(t *testing.T) {
	parsers := map[string]func(io.ReadCloser, int, *models.Query) *backend.DataResponse{
		"buffered":  buffered.ResponseParse,
		"streaming": querydata.ResponseParse,
	}
	for name, parse := range parsers {
		for _, status := range []int{400, 500} {
			for _, body := range []string{`{}`, `{"results":[]}`, `{"code":"invalid","message":"query failed"}`} {
				t.Run(fmt.Sprintf("%s/%d/%s", name, status, body), func(t *testing.T) {
					var response *backend.DataResponse
					require.NotPanics(t, func() {
						response = parse(io.NopCloser(strings.NewReader(body)), status, &models.Query{})
					})
					require.NotNil(t, response)
					require.Error(t, response.Error)
					require.Equal(t, backend.ErrorSourceFromHTTPStatus(status), response.ErrorSource)
					require.Empty(t, response.Frames)
				})
			}
		}
	}
}
