package influxdb

import (
	"fmt"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/stretchr/testify/require"
)

const raceQueryCount = 50

func TestConcurrentInfluxQLQueries(t *testing.T) {
	body := `{"results":[{"statement_id":0,"series":[{"name":"cpu","columns":["time","value"],"values":[[1622505600000,42]]}]}]}`
	ds := GetMockDataSource(influxVersionInfluxQL, RoundTripper{Body: body})

	queries := make([]backend.DataQuery, 0, raceQueryCount)
	for i := 0; i < raceQueryCount; i++ {
		queries = append(queries, backend.DataQuery{
			RefID: fmt.Sprintf("Q%d", i),
			JSON:  []byte(`{"query": "SELECT value FROM cpu", "rawQuery": true}`),
		})
	}

	resp, err := ds.QueryData(parallelContext(10), &backend.QueryDataRequest{Queries: queries})
	require.NoError(t, err)
	require.Len(t, resp.Responses, raceQueryCount)
	for refID, res := range resp.Responses {
		require.NoError(t, res.Error, "query %s failed", refID)
		require.NotEmpty(t, res.Frames, "query %s returned no frames", refID)
		require.Equal(t, 1, res.Frames[0].Fields[1].Len(), "query %s: unexpected row count", refID)
	}
}

func TestConcurrentFluxQueries(t *testing.T) {
	csv := `#datatype,string,long,dateTime:RFC3339,dateTime:RFC3339,dateTime:RFC3339,double,string,string
#group,false,false,true,true,false,false,true,true
#default,_result,,,,,,,
,result,table,_start,_stop,_time,_value,_field,_measurement
,,0,2026-06-01T00:00:00Z,2026-06-01T04:00:00Z,2026-06-01T00:00:00Z,25,temperature,sensor
`
	ds := GetMockDataSource(influxVersionFlux, RoundTripper{Body: csv})
	ds.info.Organization = "test-org"
	ds.info.Timeout = 30 * time.Second // the influxdb2 client derives its HTTP timeout from this

	queries := make([]backend.DataQuery, 0, raceQueryCount)
	for i := 0; i < raceQueryCount; i++ {
		queries = append(queries, backend.DataQuery{
			RefID:         fmt.Sprintf("Q%d", i),
			JSON:          []byte(`{"query": "from(bucket: \"fixtures\")"}`),
			MaxDataPoints: 100,
		})
	}

	resp, err := ds.QueryData(parallelContext(10), &backend.QueryDataRequest{Queries: queries})
	require.NoError(t, err)
	require.Len(t, resp.Responses, raceQueryCount)
	for refID, res := range resp.Responses {
		require.NoError(t, res.Error, "query %s failed", refID)
		require.NotEmpty(t, res.Frames, "query %s returned no frames", refID)
		require.NotEmpty(t, res.Frames[0].Fields, "query %s returned an empty frame", refID)
	}
}
