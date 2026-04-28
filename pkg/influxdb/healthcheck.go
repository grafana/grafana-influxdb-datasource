package influxdb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/grafana-plugin-sdk-go/backend/tracing"

	"github.com/grafana/grafana-influxdb-datasource/pkg/influxdb/flux"
	"github.com/grafana/grafana-influxdb-datasource/pkg/influxdb/fsql"
	"github.com/grafana/grafana-influxdb-datasource/pkg/influxdb/influxql"
	"github.com/grafana/grafana-influxdb-datasource/pkg/influxdb/models"
)

const (
	refID = "healthcheck"
)

func (ds *DataSource) CheckHealth(ctx context.Context, req *backend.CheckHealthRequest) (*backend.CheckHealthResult,
	error) {
	logger := logger.FromContext(ctx)

	if ds.info == nil {
		return getHealthCheckMessage(logger, "", errors.New("invalid datasource info received"))
	}

	switch ds.info.Version {
	case influxVersionFlux:
		return CheckFluxHealth(ctx, ds.info, req)
	case influxVersionInfluxQL:
		return CheckInfluxQLHealth(ctx, ds.info, req)
	case influxVersionSQL:
		return CheckSQLHealth(ctx, ds.info, req)
	default:
		return getHealthCheckMessage(logger, "", errors.New("unknown influx version"))
	}
}

func CheckFluxHealth(ctx context.Context, dsInfo *models.DatasourceInfo,
	req *backend.CheckHealthRequest) (*backend.CheckHealthResult,
	error) {
	logger := logger.FromContext(ctx)
	ds, err := flux.Query(ctx, dsInfo, backend.QueryDataRequest{
		PluginContext: req.PluginContext,
		Queries: []backend.DataQuery{
			{
				RefID:         refID,
				JSON:          []byte(`{ "query": "buckets()" }`),
				Interval:      1 * time.Minute,
				MaxDataPoints: 423,
				TimeRange: backend.TimeRange{
					From: time.Now().AddDate(0, 0, -1),
					To:   time.Now(),
				},
			},
		},
	})

	if err != nil {
		return getHealthCheckMessage(logger, "error performing flux query", err)
	}
	if res, ok := ds.Responses[refID]; ok {
		if res.Error != nil {
			return getHealthCheckMessage(logger, "error reading buckets", res.Error)
		}
		if len(res.Frames) > 0 && len(res.Frames[0].Fields) > 0 {
			return getHealthCheckMessage(logger, fmt.Sprintf("%d buckets found", res.Frames[0].Fields[0].Len()), nil)
		}
	}

	return getHealthCheckMessage(logger, "", errors.New("error getting flux query buckets"))
}

func CheckInfluxQLHealth(ctx context.Context, dsInfo *models.DatasourceInfo, req *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	logger := logger.FromContext(ctx)
	tracer := tracing.DefaultTracer()
	resp, err := influxql.Query(ctx, tracer, dsInfo, &backend.QueryDataRequest{
		PluginContext: req.PluginContext,
		Headers:       req.Headers,
		Queries: []backend.DataQuery{
			{
				RefID:     refID,
				QueryType: "health",
				JSON:      []byte(`{"query": "SHOW measurements", "rawQuery": true}`),
			},
		},
	})
	if err != nil {
		return getHealthCheckMessage(logger, "error performing influxQL query", err)
	}

	if res, ok := resp.Responses[refID]; ok {
		if res.Error != nil {
			return getHealthCheckMessage(logger, "error reading influxDB", res.Error)
		}

		if len(res.Frames) == 0 {
			return getHealthCheckMessage(logger, "0 measurements found", nil)
		}

		if len(res.Frames) > 0 && len(res.Frames[0].Fields) > 0 {
			return getHealthCheckMessage(logger, fmt.Sprintf("%d measurements found", res.Frames[0].Fields[0].Len()), nil)
		}
	}

	return getHealthCheckMessage(logger, "", errors.New("error connecting influxDB influxQL"))
}

func CheckSQLHealth(ctx context.Context, dsInfo *models.DatasourceInfo, req *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	ds, err := fsql.Query(ctx, dsInfo, backend.QueryDataRequest{
		PluginContext: req.PluginContext,
		Queries: []backend.DataQuery{
			{
				RefID:         refID,
				JSON:          []byte(`{ "rawSql": "select 1", "format": "table" }`),
				Interval:      1 * time.Minute,
				MaxDataPoints: 423,
				TimeRange: backend.TimeRange{
					From: time.Now().AddDate(0, 0, -1),
					To:   time.Now(),
				},
			},
		},
	})

	if err != nil {
		return getHealthCheckMessage(logger, "error performing sql query", err)
	}

	res := ds.Responses[refID]
	if res.Error != nil {
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusError,
			Message: fmt.Sprintf("ERROR: %s", res.Error),
		}, nil
	}

	return &backend.CheckHealthResult{
		Status:  backend.HealthStatusOk,
		Message: "OK",
	}, nil
}

func getHealthCheckMessage(logger log.Logger, message string, err error) (*backend.CheckHealthResult, error) {
	if err == nil {
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusOk,
			Message: fmt.Sprintf("datasource is working. %s", message),
		}, nil
	}

	logger.Warn("Error performing influxdb healthcheck", "err", err.Error())
	errorMessage := fmt.Sprintf("%s %s", err.Error(), message)

	return &backend.CheckHealthResult{
		Status:  backend.HealthStatusError,
		Message: errorMessage,
	}, nil
}
