package flux

import (
	"context"
	"errors"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/influxdata/influxdb-client-go/v2/api/http"
)

const maxPointsEnforceFactor float64 = 10

// executeQuery runs a flux query using the queryModel to interpolate the
// query and the runner to execute it, then hands the result stream to
// parseResponse. Transport failures are mapped here; everything
// response-shaped lives on the parse side.
func executeQuery(ctx context.Context, logger log.Logger, query queryModel, runner queryRunner, maxSeries int) (dr backend.DataResponse) {
	dr = backend.DataResponse{}

	flux := interpolate(query)

	logger.Debug("Executing Flux query", "flux", flux)

	tables, err := runner.runQuery(ctx, flux)
	if err != nil {
		var influxHttpError *http.Error
		if errors.As(err, &influxHttpError) {
			dr.ErrorSource = backend.ErrorSourceFromHTTPStatus(influxHttpError.StatusCode)
			dr.Status = backend.Status(influxHttpError.StatusCode)
		}
		logger.Warn("Flux query failed", "err", err, "query", flux)
		dr.Error = err
	} else {
		dr = parseResponse(logger, tables, query, maxSeries)
	}

	// Make sure there is at least one frame
	if len(dr.Frames) == 0 {
		dr.Frames = append(dr.Frames, data.NewFrame(""))
	}
	firstFrame := dr.Frames[0]
	if firstFrame.Meta == nil {
		firstFrame.SetMeta(&data.FrameMeta{})
	}
	firstFrame.Meta.ExecutedQueryString = flux
	return dr
}
