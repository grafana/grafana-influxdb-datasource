package flux

import (
	"errors"
	"fmt"
	"strings"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/influxdata/influxdb-client-go/v2/api/query"
)

// tableResult is the stream of Flux tables consumed by the parse stage. It is
// the subset of api.QueryTableResult the frame builder needs, defined here so
// parsing can be unit-tested with a fake stream.
type tableResult interface {
	Next() bool
	TableChanged() bool
	TableMetadata() *query.FluxTableMetadata
	Record() *query.FluxRecord
	Err() error
}

// parseResponse turns a Flux table stream into a data response. It owns
// everything response-shaped: frame building, error sources and the
// max-points error rewrite. Transport errors never reach it; executeQuery
// maps those before parsing starts.
func parseResponse(logger log.Logger, tables tableResult, query queryModel, maxSeries int) backend.DataResponse {
	// we only enforce a larger number than maxDataPoints
	maxPointsEnforced := int(float64(query.MaxDataPoints) * maxPointsEnforceFactor)

	dr := readDataFrames(logger, tables, maxPointsEnforced, maxSeries)

	if dr.Error != nil {
		// we check if a too-many-data-points error happened, and if it is so,
		// we improve the error-message.
		// (we have to do it in such a complicated way, because at the point where
		// the error happens, there is not enough info to create a nice error message)
		var maxPointError maxPointsExceededError
		if errors.As(dr.Error, &maxPointError) {
			errMsg := "A query returned too many datapoints and the results have been truncated at %d points to prevent memory issues. At the current graph size, Grafana can only draw %d."
			// we recommend to the user to use AggregateWindow(), but only if it is not already used
			if !strings.Contains(query.RawQuery, "aggregateWindow(") {
				errMsg += " Try using the aggregateWindow() function in your query to reduce the number of points returned."
			}

			dr.Error = fmt.Errorf(errMsg, maxPointError.Count, query.MaxDataPoints)
			dr.ErrorSource = backend.ErrorSourceDownstream
		}
	}

	return dr
}

func readDataFrames(logger log.Logger, result tableResult, maxPoints int, maxSeries int) (dr backend.DataResponse) {
	logger.Debug("Reading data frames from query result", "maxPoints", maxPoints, "maxSeries", maxSeries)
	dr = backend.DataResponse{}

	builder := &frameBuilder{
		maxPoints: maxPoints,
		maxSeries: maxSeries,
	}

	for result.Next() {
		// Observe when there is new grouping key producing new table
		if result.TableChanged() {
			if builder.frames != nil {
				for _, frame := range builder.frames {
					dr.Frames = append(dr.Frames, frame)
				}
			}
			err := builder.Init(result.TableMetadata())
			if err != nil {
				dr.Error = err
				return
			}
		}

		if builder.frames == nil {
			dr.Error = fmt.Errorf("invalid state")
			return dr
		}

		err := builder.Append(result.Record())
		if err != nil {
			dr.Error = err
			var maxSeriesError maxSeriesExceededError
			if errors.As(err, &maxSeriesError) {
				dr.ErrorSource = backend.ErrorSourceDownstream
			}
			break
		}
	}

	// Add the inprogress record
	if builder.frames != nil {
		for _, frame := range builder.frames {
			dr.Frames = append(dr.Frames, frame)
		}
	}

	// result.Err() is probably more important then the other errors
	if result.Err() != nil {
		dr.Error = result.Err()
	}
	return dr
}
