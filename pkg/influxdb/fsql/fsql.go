package fsql

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/grafana/grafana-influxdb-datasource/pkg/influxdb/models"
)

var (
	glog = backend.NewLoggerWith("logger", "tsdb.influx_flightsql")
)

type SQLOptions struct {
	Addr     string              `json:"host"`
	Metadata []map[string]string `json:"metadata"`
	Token    string              `json:"token"`
}

// Executor runs Flight SQL queries for a single request. The underlying gRPC
// connection multiplexes concurrent streams, and all per-query state is
// created inside Execute, so it is safe for concurrent use.
type Executor struct {
	client *client
}

// NewExecutor validates the datasource configuration and dials the Flight
// SQL client for this request.
func NewExecutor(dsInfo *models.DatasourceInfo) (*Executor, error) {
	if dsInfo.URL == "" {
		return nil, fmt.Errorf("missing URL from datasource configuration")
	}

	u, err := ParseURL(dsInfo.URL)
	if err != nil {
		return nil, err
	}

	md := metadata.MD{}
	if dsInfo.DbName != "" {
		md.Set("database", dsInfo.DbName)
	}
	if dsInfo.Token != "" {
		md.Set("Authorization", fmt.Sprintf("Bearer %s", dsInfo.Token))
	}

	fsqlClient, err := newFlightSQLClient(u, md, !dsInfo.InsecureGrpc, dsInfo.TLSConfig, dsInfo.ProxyClient)
	if err != nil {
		return nil, err
	}

	return &Executor{client: fsqlClient}, nil
}

// Execute runs one query and returns its response. Failures are reported
// inside the response so a bad query cannot abandon the rest of the batch.
func (e *Executor) Execute(ctx context.Context, q backend.DataQuery) backend.DataResponse {
	logger := glog.FromContext(ctx)

	if e.client.md.Len() != 0 {
		ctx = metadata.NewOutgoingContext(ctx, e.client.md)
	}

	qm, err := getQueryModel(q)
	if err != nil {
		return backend.ErrDataResponseWithSource(backend.StatusValidationFailed, backend.ErrorSourceDownstream, "bad request")
	}

	logger.Info(fmt.Sprintf("InfluxDB executing SQL: %s", qm.RawSQL))
	info, err := e.client.Execute(ctx, qm.RawSQL)
	if err != nil {
		return errorResponse(err)
	}
	if len(info.Endpoint) != 1 {
		return backend.ErrDataResponse(backend.StatusInternal, fmt.Sprintf("unsupported endpoint count in response: %d", len(info.Endpoint)))
	}

	reader, err := e.client.DoGetWithHeaderExtraction(ctx, info.Endpoint[0].Ticket)
	if err != nil {
		return backend.ErrDataResponse(backend.StatusInternal, fmt.Sprintf("flightsql: %s", err))
	}
	defer reader.Release()

	headers, err := reader.Header()
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to extract headers: %s", err))
	}

	return newQueryDataResponse(reader, *qm.Query, headers)
}

// Close releases the Flight SQL client and its gRPC connection.
func (e *Executor) Close() error {
	return e.client.Close()
}

// errorResponse maps a Flight SQL error to a data response, preserving the
// existing gRPC-code-to-status mapping.
func errorResponse(err error) backend.DataResponse {
	errStr := fmt.Sprintf("flightsql: %s", err)
	grpcStatusErr, ok := status.FromError(err)
	if !ok {
		return backend.ErrDataResponse(backend.StatusInternal, errStr)
	}
	switch grpcStatusErr.Code() {
	case codes.InvalidArgument:
		return backend.ErrDataResponseWithSource(backend.StatusBadRequest, backend.ErrorSourceDownstream, errStr)
	case codes.PermissionDenied:
		return backend.ErrDataResponseWithSource(backend.StatusForbidden, backend.ErrorSourceDownstream, errStr)
	case codes.NotFound:
		return backend.ErrDataResponseWithSource(backend.StatusNotFound, backend.ErrorSourceDownstream, errStr)
	case codes.Unavailable:
		return backend.ErrDataResponseWithSource(http.StatusServiceUnavailable, backend.ErrorSourceDownstream, errStr)
	case codes.Unauthenticated:
		return backend.ErrDataResponseWithSource(backend.StatusUnauthorized, backend.ErrorSourceDownstream, errStr)
	default:
		return backend.ErrDataResponse(backend.StatusInternal, errStr)
	}
}

func ParseURL(endpoint string) (string, error) {
	if endpoint == "" {
		return "", fmt.Errorf("missing URL from datasource configuration")
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("bad URL : %s", err)
	}

	addr := u.Host
	if u.Port() == "" {
		addr += ":443"
	}

	// If the user has specified an address with no scheme it can still be valid
	// So we use the raw URL value
	if u.Host == "" {
		addr = endpoint
	}

	return addr, nil
}
