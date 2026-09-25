package fsql

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/apache/arrow-go/v18/arrow/flight"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"google.golang.org/grpc"
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

// Executor runs Flight SQL queries for one request and is safe for concurrent use.
type Executor struct {
	client *client
}

// NewExecutor validates the configuration and dials the Flight SQL client for one request.
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

// Execute runs one query and reports any failure in the returned response.
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
	reader, errResp := runQuery(ctx, e.client, qm.RawSQL)
	if errResp != nil {
		return *errResp
	}
	defer reader.Release()

	headers, err := reader.Header()
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to extract headers: %s", err))
	}

	return newQueryDataResponse(reader, *qm.Query, headers)
}

// flightRunner is the subset of the Flight SQL client that runQuery uses.
type flightRunner interface {
	Execute(ctx context.Context, sql string, opts ...grpc.CallOption) (*flight.FlightInfo, error)
	DoGetWithHeaderExtraction(ctx context.Context, in *flight.Ticket, opts ...grpc.CallOption) (*flightReader, error)
}

// runQuery executes the SQL and opens the result stream.
// A non-nil response means the query failed and the reader is nil.
func runQuery(ctx context.Context, c flightRunner, sql string) (*flightReader, *backend.DataResponse) {
	info, err := c.Execute(ctx, sql)
	if err != nil {
		resp := errorResponse(err)
		return nil, &resp
	}
	if len(info.Endpoint) != 1 {
		resp := backend.ErrDataResponse(backend.StatusInternal, fmt.Sprintf("unsupported endpoint count in response: %d", len(info.Endpoint)))
		return nil, &resp
	}

	reader, err := c.DoGetWithHeaderExtraction(ctx, info.Endpoint[0].Ticket)
	if err != nil {
		resp := backend.ErrDataResponse(backend.StatusInternal, fmt.Sprintf("flightsql: %s", err))
		return nil, &resp
	}
	return reader, nil
}

// Close releases the Flight SQL client and its gRPC connection.
func (e *Executor) Close() error {
	return e.client.Close()
}

// errorResponse maps a Flight SQL error to a data response.
func errorResponse(err error) backend.DataResponse {
	errStr := fmt.Sprintf("flightsql: %s", err)
	grpcStatusErr, ok := status.FromError(err)
	if !ok {
		return backend.ErrDataResponse(backend.StatusInternal, errStr)
	}
	st, mapped := backendStatus(grpcStatusErr.Code())
	if !mapped {
		return backend.ErrDataResponse(backend.StatusInternal, errStr)
	}
	return backend.ErrDataResponseWithSource(st, backend.ErrorSourceDownstream, errStr)
}

// backendStatus maps a gRPC code to a plugin status.
// The bool is false when the code falls back to StatusInternal.
func backendStatus(code codes.Code) (backend.Status, bool) {
	switch code {
	case codes.InvalidArgument:
		return backend.StatusBadRequest, true
	case codes.PermissionDenied:
		return backend.StatusForbidden, true
	case codes.NotFound:
		return backend.StatusNotFound, true
	case codes.Unavailable:
		return backend.Status(http.StatusServiceUnavailable), true
	case codes.Unauthenticated:
		return backend.StatusUnauthorized, true
	default:
		return backend.StatusInternal, false
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
