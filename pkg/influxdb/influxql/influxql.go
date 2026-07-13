package influxql

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/grafana/grafana-influxdb-datasource/pkg/influxdb/influxql/buffered"
	"github.com/grafana/grafana-influxdb-datasource/pkg/influxdb/influxql/querydata"
	"github.com/grafana/grafana-influxdb-datasource/pkg/influxdb/models"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/grafana-plugin-sdk-go/config"
)

const (
	defaultRetentionPolicy = "default"
	metadataPrefix         = "x-grafana-meta-add-"
)

var (
	ErrInvalidHttpMode = errors.New("'httpMode' should be either 'GET' or 'POST'")
	ErrInvalidUrl      = errors.New("URL must contain scheme and host")
	glog               = backend.NewLoggerWith("logger", "tsdb.influx_influxql")
)

// responseParser turns an InfluxQL response body into a data response. Both
// parser packages (buffered and streaming querydata) match this signature,
// making the parsing strategy a swappable seam.
type responseParser func(io.ReadCloser, int, *models.Query) *backend.DataResponse

// Executor runs InfluxQL queries for a single request. It is safe for
// concurrent use: per-query state lives entirely inside Execute.
type Executor struct {
	dsInfo *models.DatasourceInfo
	tracer trace.Tracer
	parse  responseParser
}

// NewExecutor reads the request-scoped feature toggles once, selects the
// parsing strategy for this request and returns an executor.
func NewExecutor(ctx context.Context, tracer trace.Tracer, dsInfo *models.DatasourceInfo) (*Executor, error) {
	var parse responseParser = buffered.ResponseParse
	if config.GrafanaConfigFromContext(ctx).FeatureToggles().IsEnabled("influxqlStreamingParser") {
		glog.FromContext(ctx).Info("InfluxDB InfluxQL streaming parser enabled: ", "info")
		parse = querydata.ResponseParse
	}
	return &Executor{
		dsInfo: dsInfo,
		tracer: tracer,
		parse:  parse,
	}, nil
}

// Execute runs one query and returns its response. Failures are reported
// inside the response, never as a panic, so a bad query cannot affect the
// rest of the batch.
func (e *Executor) Execute(ctx context.Context, reqQuery backend.DataQuery) backend.DataResponse {
	logger := glog.FromContext(ctx)

	query, err := models.QueryParse(reqQuery, logger)
	if err != nil {
		return backend.DataResponse{Error: err, ErrorSource: backend.ErrorSourceDownstream}
	}

	// query.Build() unconditionally returns nil for error. Build reads the
	// time range from Queries[0], so pass the query being executed rather
	// than the whole batch.
	rawQuery, _ := query.Build(&backend.QueryDataRequest{Queries: []backend.DataQuery{reqQuery}})

	query.RefID = reqQuery.RefID
	query.RawQuery = rawQuery

	logger.Debug("Influxdb query", "raw query", rawQuery)

	request, err := createRequest(ctx, logger, e.dsInfo, rawQuery, query.Policy)
	if err != nil {
		return backend.DataResponse{Error: err, ErrorSource: backend.ErrorSourceDownstream}
	}

	res, err := e.dsInfo.HTTPClient.Do(request)
	if err != nil {
		return backend.DataResponse{Error: err, ErrorSource: backend.ErrorSourceDownstream}
	}

	return e.parseResponse(ctx, res, query)
}

// parseResponse owns everything response-shaped: it closes the body, wraps
// parsing in a span, applies the parsing strategy chosen at construction and
// stamps custom metadata headers onto the first frame.
func (e *Executor) parseResponse(ctx context.Context, res *http.Response, query *models.Query) backend.DataResponse {
	logger := glog.FromContext(ctx)
	defer func() {
		if err := res.Body.Close(); err != nil {
			logger.Warn("Failed to close response body", "err", err)
		}
	}()

	_, endSpan := startTrace(ctx, e.tracer, "datasource.influxdb.influxql.parseResponse")
	defer endSpan()

	resp := e.parse(res.Body, res.StatusCode, query)

	if len(resp.Frames) > 0 {
		resp.Frames[0].Meta.Custom = readCustomMetadata(res)
	}

	return *resp
}

// Close implements the executor contract; InfluxQL holds no per-request
// resources beyond the shared HTTP client.
func (e *Executor) Close() error {
	return nil
}

func createRequest(ctx context.Context, logger log.Logger, dsInfo *models.DatasourceInfo, queryStr string, retentionPolicy string) (*http.Request, error) {
	u, err := url.Parse(dsInfo.URL)
	if err != nil {
		return nil, err
	}

	// It's possible that the configuration is bad, and we'll have a URL
	// without a scheme or host. This is valid from the PoV of the Go std
	// library url.Parse(), but not for this data source.
	if u.Host == "" || u.Scheme == "" {
		return nil, ErrInvalidUrl
	}

	u.Path = path.Join(u.Path, "query")
	httpMode := dsInfo.HTTPMode

	var req *http.Request
	switch httpMode {
	case "GET":
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
	case "POST":
		bodyValues := url.Values{}
		bodyValues.Add("q", queryStr)
		body := bodyValues.Encode()
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, u.String(), strings.NewReader(body))
		if err != nil {
			return nil, err
		}
	default:
		return nil, ErrInvalidHttpMode
	}

	params := req.URL.Query()
	params.Set("db", dsInfo.DbName)
	params.Set("epoch", "ms")
	// default is hardcoded default retention policy
	// InfluxDB will use the default policy when it is not added to the request
	if retentionPolicy != "" && retentionPolicy != "default" {
		params.Set("rp", retentionPolicy)
	}

	switch httpMode {
	case "GET":
		params.Set("q", queryStr)
	case "POST":
		req.Header.Set("Content-type", "application/x-www-form-urlencoded")
	}

	req.URL.RawQuery = params.Encode()

	logger.Debug("Influxdb request", "url", req.URL.String())
	return req, nil
}

func readCustomMetadata(res *http.Response) map[string]any {
	var result map[string]any
	for k := range res.Header {
		if key, found := strings.CutPrefix(strings.ToLower(k), metadataPrefix); found {
			if result == nil {
				result = make(map[string]any)
			}
			result[key] = res.Header.Get(k)
		}
	}
	return result
}

// startTrace setups a trace but does not panic if tracer is nil which helps with testing
func startTrace(ctx context.Context, tracer trace.Tracer, name string, attributes ...attribute.KeyValue) (context.Context, func()) {
	if tracer == nil {
		return ctx, func() {}
	}
	ctx, span := tracer.Start(ctx, name, trace.WithAttributes(attributes...))
	return ctx, func() {
		span.End()
	}
}
