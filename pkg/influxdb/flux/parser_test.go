package flux

import (
	"errors"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/influxdata/influxdb-client-go/v2/api/query"
	"github.com/stretchr/testify/require"
)

// fakeRow is one Next() step of a fakeTableResult.
type fakeRow struct {
	tableChanged bool
	metadata     *query.FluxTableMetadata
	record       *query.FluxRecord
}

// fakeTableResult drives parseResponse through arbitrary stream shapes,
// including ones the CSV parser can never produce, without a client or an
// HTTP server.
type fakeTableResult struct {
	rows []fakeRow
	pos  int
	err  error
}

func (f *fakeTableResult) Next() bool {
	if f.pos < len(f.rows) {
		f.pos++
		return true
	}
	return false
}

func (f *fakeTableResult) TableChanged() bool                      { return f.rows[f.pos-1].tableChanged }
func (f *fakeTableResult) TableMetadata() *query.FluxTableMetadata { return f.rows[f.pos-1].metadata }
func (f *fakeTableResult) Record() *query.FluxRecord               { return f.rows[f.pos-1].record }
func (f *fakeTableResult) Err() error                              { return f.err }

// simpleMetadata describes the usual _time/_value shape with one group
// column ("host") whose value changes start a new table.
func simpleMetadata() *query.FluxTableMetadata {
	return query.NewFluxTableMetadataFull(0, []*query.FluxColumn{
		query.NewFluxColumnFull(stringDatatype, "", "result", false, 0),
		query.NewFluxColumnFull(longDatatype, "", "table", false, 1),
		query.NewFluxColumnFull(datetimeRFC339DataType, "", "_time", false, 2),
		query.NewFluxColumnFull(doubleDatatype, "", "_value", false, 3),
		query.NewFluxColumnFull(stringDatatype, "", "host", true, 4),
	})
}

func simpleRecord(host string, value float64) *query.FluxRecord {
	return query.NewFluxRecord(0, map[string]any{
		"_time":  time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
		"_value": value,
		"host":   host,
	})
}

func TestParseResponse(t *testing.T) {
	tests := []struct {
		name       string
		result     *fakeTableResult
		query      queryModel
		maxSeries  int
		wantFrames int
		wantErr    string
		wantSource backend.ErrorSource
	}{
		{
			name: "groups rows into one frame per table",
			result: &fakeTableResult{rows: []fakeRow{
				{tableChanged: true, metadata: simpleMetadata(), record: simpleRecord("a", 1)},
				{record: simpleRecord("a", 2)},
				{record: simpleRecord("b", 3)},
			}},
			query:      queryModel{MaxDataPoints: 100},
			maxSeries:  10,
			wantFrames: 2,
		},
		{
			name: "record before metadata is an invalid state",
			result: &fakeTableResult{rows: []fakeRow{
				{tableChanged: false, record: simpleRecord("a", 1)},
			}},
			query:     queryModel{MaxDataPoints: 100},
			maxSeries: 10,
			wantErr:   "invalid state",
		},
		{
			name: "stream error wins over parsed data",
			result: &fakeTableResult{
				rows: []fakeRow{
					{tableChanged: true, metadata: simpleMetadata(), record: simpleRecord("a", 1)},
				},
				err: errors.New("stream torn down"),
			},
			query:     queryModel{MaxDataPoints: 100},
			maxSeries: 10,
			wantErr:   "stream torn down",
		},
		{
			name: "max series exceeded is a downstream error",
			result: &fakeTableResult{rows: []fakeRow{
				{tableChanged: true, metadata: simpleMetadata(), record: simpleRecord("a", 1)},
				{record: simpleRecord("b", 2)},
			}},
			query:      queryModel{MaxDataPoints: 100},
			maxSeries:  1,
			wantErr:    "max series limit exceeded",
			wantSource: backend.ErrorSourceDownstream,
		},
		{
			name: "max points exceeded recommends aggregateWindow",
			result: &fakeTableResult{rows: []fakeRow{
				{tableChanged: true, metadata: simpleMetadata(), record: simpleRecord("a", 1)},
			}},
			query:      queryModel{MaxDataPoints: 0, RawQuery: `from(bucket: "x")`},
			maxSeries:  10,
			wantErr:    "aggregateWindow",
			wantSource: backend.ErrorSourceDownstream,
		},
		{
			name: "max points exceeded with aggregateWindow already in use",
			result: &fakeTableResult{rows: []fakeRow{
				{tableChanged: true, metadata: simpleMetadata(), record: simpleRecord("a", 1)},
			}},
			query:      queryModel{MaxDataPoints: 0, RawQuery: `aggregateWindow(every: 1m)`},
			maxSeries:  10,
			wantErr:    "truncated",
			wantSource: backend.ErrorSourceDownstream,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dr := parseResponse(glog, tt.result, tt.query, tt.maxSeries)

			if tt.wantErr != "" {
				require.Error(t, dr.Error)
				require.Contains(t, dr.Error.Error(), tt.wantErr)
				require.Equal(t, tt.wantSource, dr.ErrorSource)
				return
			}

			require.NoError(t, dr.Error)
			require.Len(t, dr.Frames, tt.wantFrames)
		})
	}
}

func TestParseResponseAggregateWindowNotRecommendedTwice(t *testing.T) {
	result := &fakeTableResult{rows: []fakeRow{
		{tableChanged: true, metadata: simpleMetadata(), record: simpleRecord("a", 1)},
	}}
	dr := parseResponse(glog, result, queryModel{MaxDataPoints: 0, RawQuery: `aggregateWindow(every: 1m)`}, 10)

	require.Error(t, dr.Error)
	require.NotContains(t, dr.Error.Error(), "aggregateWindow")
}
