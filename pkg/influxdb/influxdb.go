package influxdb

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/httpclient"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/grafana-plugin-sdk-go/backend/tracing"

	"github.com/grafana/grafana-influxdb-datasource/pkg/influxdb/flux"
	"github.com/grafana/grafana-influxdb-datasource/pkg/influxdb/fsql"
	"github.com/grafana/grafana-influxdb-datasource/pkg/influxdb/influxql"
	"github.com/grafana/grafana-influxdb-datasource/pkg/influxdb/models"
)

var logger log.Logger = backend.NewLoggerWith("logger", "tsdb.influxdb")

type DataSource struct {
	info *models.DatasourceInfo
}

func NewDatasource(ctx context.Context, settings backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
	opts, err := settings.HTTPClientOptions(ctx)
	if err != nil {
		return nil, err
	}

	client, err := httpclient.NewProvider().New(opts)
	if err != nil {
		return nil, err
	}

	jsonData := models.DatasourceInfo{}
	err = json.Unmarshal(settings.JSONData, &jsonData)
	if err != nil {
		return nil, fmt.Errorf("error reading settings: %w", err)
	}

	httpMode := jsonData.HTTPMode
	if httpMode == "" {
		httpMode = "GET"
	}

	maxSeries := jsonData.MaxSeries
	if maxSeries == 0 {
		maxSeries = 1000
	}

	version := jsonData.Version
	if version == "" {
		version = influxVersionInfluxQL
	}

	database := jsonData.DbName
	if database == "" {
		database = settings.Database
	}

	proxyClient, err := settings.ProxyClient(ctx)
	if err != nil {
		logger.Error("influx proxy creation failed", "error", err)
		return nil, fmt.Errorf("influx proxy creation failed")
	}

	return &DataSource{
		info: &models.DatasourceInfo{
			HTTPClient:    client,
			URL:           settings.URL,
			DbName:        database,
			Version:       version,
			HTTPMode:      httpMode,
			TimeInterval:  jsonData.TimeInterval,
			DefaultBucket: jsonData.DefaultBucket,
			Organization:  jsonData.Organization,
			MaxSeries:     maxSeries,
			InsecureGrpc:  jsonData.InsecureGrpc,
			Token:         settings.DecryptedSecureJSONData["token"],
			Timeout:       opts.Timeouts.Timeout,
			ProxyClient:   proxyClient,
			TLSConfig:     opts.TLS,
		},
	}, nil
}

func (ds *DataSource) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	logger := logger.FromContext(ctx)
	logger.Debug("Received a query request", "numQueries", len(req.Queries))

	tracer := tracing.DefaultTracer()

	logger.Debug(fmt.Sprintf("Making a %s type query", ds.info.Version))

	switch ds.info.Version {
	case influxVersionFlux:
		return flux.Query(ctx, ds.info, *req)
	case influxVersionInfluxQL:
		return influxql.Query(ctx, tracer, ds.info, req)
	case influxVersionSQL:
		return fsql.Query(ctx, ds.info, *req)
	default:
		return nil, fmt.Errorf("unknown influxdb version")
	}
}
