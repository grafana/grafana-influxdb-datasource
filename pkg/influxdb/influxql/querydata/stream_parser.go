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

// excerptLimit is the maximum number of body bytes retained for an error excerpt.
const excerptLimit = 200

// cappedBuffer keeps at most excerptLimit bytes, so a streamed response is not
// held in memory in full just to produce an error excerpt.
type cappedBuffer struct {
	buf []byte
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if len(c.buf) < excerptLimit {
		n := excerptLimit - len(c.buf)
		if n > len(p) {
			n = len(p)
		}
		c.buf = append(c.buf, p[:n]...)
	}
	return len(p), nil
}

func (c *cappedBuffer) Bytes() []byte { return c.buf }

// readErrReader wraps an io.Reader and records the last non-EOF read error.
// jsoniter swaps io.EOF/io.ErrUnexpectedEOF for its own parse error, so the
// underlying read error is otherwise lost; this preserves it for callers.
type readErrReader struct {
	r   io.Reader
	err error
}

func (e *readErrReader) Read(p []byte) (int, error) {
	n, err := e.r.Read(p)
	if err != nil && err != io.EOF {
		e.err = err
	}
	return n, err
}

func ResponseParse(buf io.ReadCloser, statusCode int, query *models.Query) *backend.DataResponse {
	defer func() {
		if err := buf.Close(); err != nil {
			fmt.Println("Failed to close response body", "err", err)
		}
	}()

	// Non-2xx responses: read the body so we can surface a useful excerpt.
	// (These are error paths, so buffering the (usually small) body is fine.)
	if statusCode/100 != 2 {
		body, readErr := io.ReadAll(buf)
		if readErr != nil {
			return &backend.DataResponse{
				Error:       fmt.Errorf("InfluxDB returned error: HTTP %d %s: %w", statusCode, http.StatusText(statusCode), readErr),
				ErrorSource: backend.ErrorSourceFromHTTPStatus(statusCode),
			}
		}

		iter := jsoniter.Parse(jsoniter.ConfigDefault, bytes.NewReader(body), 1024)
		r := converter.ReadInfluxQLStyleResult(iter, query)

		// Only treat r.Error as a real InfluxDB application error when the body
		// parsed cleanly. A non-JSON body (e.g. a 502 HTML page) puts a parse
		// error in r.Error, which must not shadow the HTTP status + excerpt.
		if r != nil && r.Error != nil && iter.Error == nil {
			return &backend.DataResponse{
				Error:       fmt.Errorf("InfluxDB returned error: %s", r.Error),
				ErrorSource: backend.ErrorSourceFromHTTPStatus(statusCode),
			}
		}
		return &backend.DataResponse{
			Error:       formatHTTPError(statusCode, body),
			ErrorSource: backend.ErrorSourceFromHTTPStatus(statusCode),
		}
	}

	// 2xx responses: stream directly from buf (do not buffer the whole body).
	// Only a capped prefix is retained, for the error excerpt.
	var excerpt cappedBuffer
	rr := &readErrReader{r: buf}
	tee := io.TeeReader(rr, &excerpt)

	iter := jsoniter.Parse(jsoniter.ConfigDefault, tee, 1024)
	r := converter.ReadInfluxQLStyleResult(iter, query)

	if r == nil {
		if iter.Error != nil {
			return &backend.DataResponse{
				Error:       fmt.Errorf("InfluxDB returned error: %w", iter.Error),
				ErrorSource: backend.ErrorSourceDownstream,
			}
		}
		return &backend.DataResponse{
			Error:       formatHTTPError(statusCode, excerpt.Bytes()),
			ErrorSource: backend.ErrorSourceDownstream,
		}
	}

	if r.Error != nil {
		// A 2xx body that isn't JSON lands here too (the converter returns a
		// non-nil r carrying the parse error). In that case keep the HTTP
		// status + excerpt text instead of the raw jsoniter error.
		if iter.Error != nil {
			// Preserve the underlying cause only when the read itself failed
			// (truncated body), wrapping it beside the captured excerpt. jsoniter
			// swaps io.EOF/io.ErrUnexpectedEOF for its own parse error, so rely
			// on the recorded read error instead of iter.Error.
			if rr.err != nil {
				ex := strings.TrimSpace(string(excerpt.Bytes()))
				if ex != "" {
					return &backend.DataResponse{
						Error:       fmt.Errorf("%w: %s", rr.err, ex),
						ErrorSource: backend.ErrorSourceDownstream,
					}
				}
				return &backend.DataResponse{
					Error:       fmt.Errorf("%w", rr.err),
					ErrorSource: backend.ErrorSourceDownstream,
				}
			}
			return &backend.DataResponse{
				Error:       formatHTTPError(statusCode, excerpt.Bytes()),
				ErrorSource: backend.ErrorSourceDownstream,
			}
		}
		// Clean parse with an application-level error: preserve it.
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
