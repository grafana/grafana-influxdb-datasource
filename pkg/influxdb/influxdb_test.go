package influxdb

import (
	"context"
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/stretchr/testify/require"
)

func TestQueryDataInfluxQLThroughFanOut(t *testing.T) {
	body := `{"results":[{"statement_id":0,"series":[{"name":"cpu","columns":["time","value"],"values":[[1622505600000,42]]}]}]}`
	ds := GetMockDataSource(influxVersionInfluxQL, RoundTripper{Body: body})

	queries := []backend.DataQuery{
		{RefID: "A", JSON: []byte(`{"query": "SELECT value FROM cpu", "rawQuery": true}`)},
		{RefID: "B", JSON: []byte(`{"query": "SELECT value FROM mem", "rawQuery": true}`)},
		{RefID: "C", JSON: []byte(`{invalid`)},
	}

	tests := []struct {
		name string
		ctx  context.Context
	}{
		{name: "serial", ctx: context.Background()},
		{name: "parallel", ctx: parallelContext(4)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := ds.QueryData(tc.ctx, &backend.QueryDataRequest{Queries: queries})
			require.NoError(t, err)
			require.Len(t, resp.Responses, 3)
			require.NoError(t, resp.Responses["A"].Error)
			require.NoError(t, resp.Responses["B"].Error)
			require.Error(t, resp.Responses["C"].Error)
		})
	}
}

func TestQueryDataUnknownVersion(t *testing.T) {
	ds := GetMockDataSource("not-a-version", RoundTripper{})
	_, err := ds.QueryData(context.Background(), &backend.QueryDataRequest{})
	require.Error(t, err)
}
