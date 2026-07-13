package flux

import (
	"context"
	"fmt"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"

	"github.com/grafana/grafana-influxdb-datasource/pkg/influxdb/models"
)

var (
	glog = backend.NewLoggerWith("logger", "tsdb.influx_flux")
)

// Executor runs Flux queries for a single request. The influxdb2 client is
// safe for concurrent use; each Execute call performs an independent HTTP
// request and consumes its own result stream.
type Executor struct {
	runner queryRunner
	client influxdb2.Client
	dsInfo *models.DatasourceInfo
}

// NewExecutor validates the datasource configuration and builds the shared
// influxdb2 client for this request.
func NewExecutor(dsInfo *models.DatasourceInfo) (*Executor, error) {
	r, err := runnerFromDataSource(dsInfo)
	if err != nil {
		return nil, err
	}
	return &Executor{runner: r, client: r.client, dsInfo: dsInfo}, nil
}

// Execute runs one query and returns its response. Failures are reported
// inside the response so a bad query cannot affect the rest of the batch.
func (e *Executor) Execute(ctx context.Context, query backend.DataQuery) backend.DataResponse {
	logger := glog.FromContext(ctx)

	qm, err := getQueryModel(query, query.TimeRange, e.dsInfo)
	if err != nil {
		return backend.DataResponse{Error: err, ErrorSource: backend.ErrorSourceDownstream}
	}

	// If the default changes also update labels/placeholder in config page.
	return executeQuery(ctx, logger, *qm, e.runner, e.dsInfo.MaxSeries)
}

// Close releases the influxdb2 client. Nil-safe so unit tests can build an
// Executor around a fake runner without a real client.
func (e *Executor) Close() error {
	if e.client != nil {
		e.client.Close()
	}
	return nil
}

// Query is a temporary wrapper kept until the dispatcher moves to
// NewExecutor/Execute.
func Query(ctx context.Context, dsInfo *models.DatasourceInfo, tsdbQuery backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	logger := glog.FromContext(ctx)
	tRes := backend.NewQueryDataResponse()
	executor, err := NewExecutor(dsInfo)
	if err != nil {
		return &backend.QueryDataResponse{}, err
	}
	defer func() {
		if err := executor.Close(); err != nil {
			logger.Warn("Failed to close flux client", "err", err)
		}
	}()

	for _, query := range tsdbQuery.Queries {
		tRes.Responses[query.RefID] = executor.Execute(ctx, query)
	}
	return tRes, nil
}

// runner is an influxdb2 Client with an attached org property and is used
// for running flux queries.
type runner struct {
	client influxdb2.Client
	org    string
}

// This is an interface to help testing
type queryRunner interface {
	runQuery(ctx context.Context, q string) (*api.QueryTableResult, error)
}

// runQuery executes fluxQuery against the Runner's organization and returns a Flux typed result.
func (r *runner) runQuery(ctx context.Context, fluxQuery string) (*api.QueryTableResult, error) {
	qa := r.client.QueryAPI(r.org)
	return qa.Query(ctx, fluxQuery)
}

// runnerFromDataSource creates a runner from the datasource model (the datasource instance's configuration).
func runnerFromDataSource(dsInfo *models.DatasourceInfo) (*runner, error) {
	org := dsInfo.Organization
	if org == "" {
		return nil, fmt.Errorf("missing organization in datasource configuration")
	}

	url := dsInfo.URL
	if url == "" {
		return nil, fmt.Errorf("missing URL from datasource configuration")
	}
	opts := influxdb2.DefaultOptions()
	opts.HTTPOptions().SetHTTPClient(dsInfo.HTTPClient)
	opts.SetHTTPRequestTimeout(uint(dsInfo.Timeout.Seconds()))
	return &runner{
		client: influxdb2.NewClientWithOptions(url, dsInfo.Token, opts),
		org:    org,
	}, nil
}
