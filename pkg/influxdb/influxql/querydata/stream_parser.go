package querydata

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	jsoniter "github.com/json-iterator/go"

	"github.com/grafana/grafana-influxdb-datasource/pkg/influxdb/influxql/converter"
	"github.com/grafana/grafana-influxdb-datasource/pkg/influxdb/influxql/util"
	"github.com/grafana/grafana-influxdb-datasource/pkg/influxdb/models"
)

func ResponseParse(buf io.ReadCloser, statusCode int, query *models.Query) *backend.DataResponse {
	defer func() {
		if err := buf.Close(); err != nil {
			fmt.Println("Failed to close response body", "err", err)
		}
	}()

	body, readErr := io.ReadAll(buf)
	if readErr != nil {
		return &backend.DataResponse{
			Error:       fmt.Errorf("InfluxDB returned error: HTTP %d %s", statusCode, http.StatusText(statusCode)),
			ErrorSource: backend.ErrorSourceFromHTTPStatus(statusCode),
		}
	}

	iter := jsoniter.Parse(jsoniter.ConfigDefault, bytes.NewReader(body), 1024)
	r := converter.ReadInfluxQLStyleResult(iter, query)

	if statusCode/100 != 2 {
		if r == nil || r.Error == nil || !jsoniter.Valid(body) {
			return &backend.DataResponse{
				Error:       formatHTTPError(statusCode, body),
				ErrorSource: backend.ErrorSourceFromHTTPStatus(statusCode),
			}
		}
		return &backend.DataResponse{
			Error:       fmt.Errorf("InfluxDB returned error: %s", r.Error),
			ErrorSource: backend.ErrorSourceFromHTTPStatus(statusCode),
		}
	}

	if r == nil || !jsoniter.Valid(body) {
		return &backend.DataResponse{
			Error:       formatHTTPError(statusCode, body),
			ErrorSource: backend.ErrorSourceDownstream,
		}
	}

	if r.Error != nil {
		return r
	}

	// The ExecutedQueryString can be viewed in QueryInspector in UI
	for i, frame := range r.Frames {
		if i == 0 {
			frame.Meta = &data.FrameMeta{ExecutedQueryString: query.RawQuery, PreferredVisualization: util.GetVisType(query.ResultFormat)}
		}
	}

	return r
}

func formatHTTPError(statusCode int, body []byte) error {
	excerpt := strings.TrimSpace(string(body))
	if len(excerpt) > 200 {
		excerpt = excerpt[:200]
	}
	if excerpt == "" {
		return fmt.Errorf("InfluxDB returned error: HTTP %d %s", statusCode, http.StatusText(statusCode))
	}
	return fmt.Errorf("InfluxDB returned error: HTTP %d %s: %s", statusCode, http.StatusText(statusCode), excerpt)
}
