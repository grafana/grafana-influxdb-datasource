package flux

import (
	"context"
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-influxdb-datasource/pkg/influxdb/models"
)

func TestExecutorExecuteBadJSON(t *testing.T) {
	executor := &Executor{
		runner: nil, // Execute must fail before touching the runner
		dsInfo: &models.DatasourceInfo{MaxSeries: 10},
	}

	res := executor.Execute(context.Background(), backend.DataQuery{RefID: "A", JSON: []byte(`{invalid`)})

	require.Error(t, res.Error)
	require.Equal(t, backend.ErrorSourceDownstream, res.ErrorSource)
}

func TestNewExecutorRequiresOrganization(t *testing.T) {
	_, err := NewExecutor(&models.DatasourceInfo{URL: "http://localhost:8086"})
	require.Error(t, err)
}

func TestNewExecutorAndClose(t *testing.T) {
	executor, err := NewExecutor(&models.DatasourceInfo{
		URL:          "http://localhost:8086",
		Organization: "grafana",
	})
	require.NoError(t, err)
	require.NoError(t, executor.Close())
}

func TestExecutorExecuteHappyPath(t *testing.T) {
	executor := &Executor{
		runner: &MockRunner{testDataPath: "simple.csv"},
		dsInfo: &models.DatasourceInfo{MaxSeries: 10},
	}
	res := executor.Execute(context.Background(), backend.DataQuery{
		RefID: "A",
		JSON:  []byte(`{"query": "from(bucket: \"b\")"}`),
	})
	require.NoError(t, res.Error)
	require.NotEmpty(t, res.Frames)
}
