// Package integration exercises the full backend QueryData path against the
// real InfluxDB version matrix from docker-compose.yaml. Tests skip when the
// stack is not running, per docs/testing/integration-testing.md in the
// data-sources repository.
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/config"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-influxdb-datasource/pkg/influxdb"
)

// Key read by config.GrafanaCfg.FeatureToggles(); only exported from the
// SDK's experimental featuretoggles package, so repeated here.
const enabledFeaturesKey = "GF_INSTANCE_FEATURE_TOGGLES_ENABLE"

// Fixture window, see tests/fixtures/README.md.
var (
	fixtureFrom = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	fixtureTo   = time.Date(2026, 6, 1, 4, 0, 0, 0, time.UTC)
)

type instance struct {
	name     string
	addr     string // host:port used for the reachability check
	settings backend.DataSourceInstanceSettings
	queries  []backend.DataQuery
	invalid  backend.DataQuery // a query that must fail on this instance
}

func requireReachable(t *testing.T, addr string) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		t.Skipf("influxdb not reachable at %s; run docker compose up -d (see docker-compose.yaml)", addr)
	}
	_ = conn.Close()
}

func newDataSource(t *testing.T, settings backend.DataSourceInstanceSettings) *influxdb.DataSource {
	t.Helper()
	inst, err := influxdb.NewDatasource(context.Background(), settings)
	require.NoError(t, err)
	ds, ok := inst.(*influxdb.DataSource)
	require.True(t, ok, "NewDatasource returned unexpected type %T", inst)
	return ds
}

func settingsJSON(t *testing.T, m map[string]any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(m)
	require.NoError(t, err)
	return b
}

func serialContext() context.Context {
	return config.WithGrafanaConfig(context.Background(), config.NewGrafanaCfg(map[string]string{}))
}

func parallelContext() context.Context {
	return config.WithGrafanaConfig(context.Background(), config.NewGrafanaCfg(map[string]string{
		enabledFeaturesKey:          "influxdbRunQueriesInParallel",
		config.ConcurrentQueryCount: "10",
	}))
}

func influxqlQuery(refID, q string) backend.DataQuery {
	return backend.DataQuery{
		RefID:         refID,
		JSON:          []byte(fmt.Sprintf(`{"query": %q, "rawQuery": true, "resultFormat": "time_series"}`, q)),
		TimeRange:     backend.TimeRange{From: fixtureFrom, To: fixtureTo},
		MaxDataPoints: 1000,
		Interval:      time.Minute,
	}
}

func fluxQuery(refID, q string) backend.DataQuery {
	return backend.DataQuery{
		RefID:         refID,
		JSON:          []byte(fmt.Sprintf(`{"query": %q}`, q)),
		TimeRange:     backend.TimeRange{From: fixtureFrom, To: fixtureTo},
		MaxDataPoints: 1000,
		Interval:      time.Minute,
	}
}

func sqlQuery(refID, q string) backend.DataQuery {
	return backend.DataQuery{
		RefID:         refID,
		JSON:          []byte(fmt.Sprintf(`{"rawSql": %q, "format": "table"}`, q)),
		TimeRange:     backend.TimeRange{From: fixtureFrom, To: fixtureTo},
		MaxDataPoints: 1000,
		Interval:      time.Minute,
	}
}

// instances mirrors provisioning/datasources/datasources.yml.
func instances(t *testing.T) []instance {
	// The sensor fixture only has readings_temperatureCelsius (see
	// tests/fixtures/sensor.lp); there is no bare "temperature" field.
	timeFilter := "time >= '2026-06-01T00:00:00Z' AND time < '2026-06-01T04:00:00Z'"
	influxqlBatch := []backend.DataQuery{
		influxqlQuery("A", fmt.Sprintf(`SELECT count("readings_temperatureCelsius") FROM "sensor" WHERE %s`, timeFilter)),
		influxqlQuery("B", fmt.Sprintf(`SELECT count(*) FROM "httplogs" WHERE %s`, timeFilter)),
		influxqlQuery("C", fmt.Sprintf(`SELECT count(*) FROM "infra" WHERE %s`, timeFilter)),
	}
	influxqlInvalid := influxqlQuery("BAD", "SELECT FROM WHERE")

	fluxBatch := []backend.DataQuery{
		fluxQuery("A", `from(bucket: "fixtures") |> range(start: 2026-06-01T00:00:00Z, stop: 2026-06-01T04:00:00Z) |> filter(fn: (r) => r._measurement == "sensor") |> count()`),
		fluxQuery("B", `from(bucket: "fixtures") |> range(start: 2026-06-01T00:00:00Z, stop: 2026-06-01T04:00:00Z) |> filter(fn: (r) => r._measurement == "httplogs") |> count()`),
		fluxQuery("C", `from(bucket: "fixtures") |> range(start: 2026-06-01T00:00:00Z, stop: 2026-06-01T04:00:00Z) |> filter(fn: (r) => r._measurement == "infra") |> count()`),
	}
	fluxInvalid := fluxQuery("BAD", "this is not flux(")

	sqlBatch := []backend.DataQuery{
		sqlQuery("A", fmt.Sprintf("SELECT COUNT(*) AS c FROM sensor WHERE %s", timeFilter)),
		sqlQuery("B", fmt.Sprintf("SELECT COUNT(*) AS c FROM httplogs WHERE %s", timeFilter)),
		sqlQuery("C", fmt.Sprintf("SELECT COUNT(*) AS c FROM infra WHERE %s", timeFilter)),
	}
	sqlInvalid := sqlQuery("BAD", "SELECT FROM WHERE")

	return []instance{
		{
			name: "v1-influxql",
			addr: "localhost:8086",
			settings: backend.DataSourceInstanceSettings{
				Type:     "influxdb",
				URL:      "http://localhost:8086",
				JSONData: settingsJSON(t, map[string]any{"version": "InfluxQL", "dbName": "fixtures", "httpMode": "POST"}),
			},
			queries: influxqlBatch,
			invalid: influxqlInvalid,
		},
		{
			name: "v2-flux",
			addr: "localhost:8087",
			settings: backend.DataSourceInstanceSettings{
				Type:                    "influxdb",
				URL:                     "http://localhost:8087",
				JSONData:                settingsJSON(t, map[string]any{"version": "Flux", "organization": "grafana", "defaultBucket": "fixtures"}),
				DecryptedSecureJSONData: map[string]string{"token": "influxdb2-admin-token"},
			},
			queries: fluxBatch,
			invalid: fluxInvalid,
		},
		{
			name: "v2-influxql",
			addr: "localhost:8087",
			settings: backend.DataSourceInstanceSettings{
				Type: "influxdb",
				URL:  "http://localhost:8087",
				JSONData: settingsJSON(t, map[string]any{
					"version": "InfluxQL", "dbName": "fixtures", "httpMode": "POST",
					"httpHeaderName1": "Authorization",
				}),
				DecryptedSecureJSONData: map[string]string{"httpHeaderValue1": "Token influxdb2-admin-token"},
			},
			queries: influxqlBatch,
			invalid: influxqlInvalid,
		},
		{
			name: "v3-sql",
			addr: "localhost:8181",
			settings: backend.DataSourceInstanceSettings{
				Type:     "influxdb",
				URL:      "http://localhost:8181",
				JSONData: settingsJSON(t, map[string]any{"version": "SQL", "dbName": "fixtures", "insecureGrpc": true}),
			},
			queries: sqlBatch,
			invalid: sqlInvalid,
		},
		{
			name: "v3-influxql",
			addr: "localhost:8181",
			settings: backend.DataSourceInstanceSettings{
				Type:     "influxdb",
				URL:      "http://localhost:8181",
				JSONData: settingsJSON(t, map[string]any{"version": "InfluxQL", "dbName": "fixtures", "httpMode": "POST"}),
			},
			queries: influxqlBatch,
			invalid: influxqlInvalid,
		},
	}
}
