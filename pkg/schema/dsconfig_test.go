package schema_test

import (
	_ "embed"
	"testing"

	"github.com/grafana/dsconfig/schema"
	"github.com/grafana/grafana-plugin-sdk-go/experimental/pluginschema"
	"k8s.io/kube-openapi/pkg/spec3"

	"github.com/grafana/grafana-influxdb-datasource/pkg/influxdb/models"
)

//go:embed dsconfig.json
var configSchemaJSON []byte

// NewDatasource reads the embedded model. Grafana, the plugin SDK, and the
// configuration and query editors read the remaining jsonData fields.
type settings struct {
	models.DatasourceInfo
	Product                  string   `json:"product"`
	ShowTagTime              string   `json:"showTagTime"`
	PDCInjected              bool     `json:"pdcInjected"`
	EnableSecureSocksProxy   bool     `json:"enableSecureSocksProxy"`
	SecureSocksProxyUsername string   `json:"secureSocksProxyUsername"`
	KeepCookies              []string `json:"keepCookies"`
	HTTPTimeout              int      `json:"timeout"`
	OAuthPassThru            bool     `json:"oauthPassThru"`
	TLSAuth                  bool     `json:"tlsAuth"`
	TLSAuthWithCACert        bool     `json:"tlsAuthWithCACert"`
	TLSSkipVerify            bool     `json:"tlsSkipVerify"`
	ServerName               string   `json:"serverName"`
}

//go:generate go test -run TestPlugin -generateArtifacts
func TestPlugin(t *testing.T) {
	schema.RunPluginTests(t, schema.PluginUnderTest{
		ID:                "influxdb",
		ConfigSchemaJSON:  configSchemaJSON,
		SettingsJSONModel: settings{},
		SecureKeys:        []string{"token", "password", "basicAuthPassword", "tlsCACert", "tlsClientCert", "tlsClientKey", "secureSocksProxyPassword"},
		SettingsExamples:  &pluginschema.SettingsExamples{Examples: examples()},
	})
}

func examples() map[string]*spec3.Example {
	type config = map[string]any
	type secrets = map[string]string

	localURL := "http://localhost:8086"
	tlsURL := "https://localhost:8086"
	return map[string]*spec3.Example{
		"influxql": example("influxql", "InfluxQL without authentication", localURL,
			config{"product": "InfluxDB OSS 1.x", "version": "InfluxQL", "dbName": "site"}, nil, nil),
		"influxql-password": example("influxql-password", "InfluxQL with database credentials", localURL,
			config{"product": "InfluxDB OSS 1.x", "version": "InfluxQL", "dbName": "site", "httpMode": "GET"},
			config{"user": "grafana"}, secrets{"password": "<password>"}),
		"influxql-http-basic": example("influxql-http-basic", "InfluxQL with HTTP basic authentication", tlsURL,
			config{"product": "InfluxDB OSS 1.x", "version": "InfluxQL", "dbName": "site"},
			config{"basicAuth": true, "basicAuthUser": "grafana"}, secrets{"basicAuthPassword": "<password>"}),
		"influxql-oauth": example("influxql-oauth", "InfluxQL with the querying user's OAuth identity", tlsURL,
			config{"product": "InfluxDB OSS 1.x", "version": "InfluxQL", "dbName": "site", "oauthPassThru": true}, nil, nil),
		"influxql-client-cert": example("influxql-client-cert", "InfluxQL with TLS client authentication and a private CA", tlsURL,
			config{"product": "InfluxDB OSS 1.x", "version": "InfluxQL", "dbName": "site", "tlsAuth": true, "tlsAuthWithCACert": true}, nil,
			secrets{"tlsClientCert": "<PEM client certificate>", "tlsClientKey": "<PEM private key>", "tlsCACert": "<PEM CA certificate>"}),
		"influxql-token": example("influxql-token", "InfluxQL with an InfluxDB 2.x API token", localURL,
			config{"product": "InfluxDB OSS 2.x", "version": "InfluxQL", "dbName": "site", "httpHeaderName1": "Authorization"},
			nil, secrets{"httpHeaderValue1": "Token <token>"}),
		"flux": example("flux", "Flux with an InfluxDB 2.x API token", localURL,
			config{"product": "InfluxDB OSS 2.x", "version": "Flux", "organization": "organization", "defaultBucket": "bucket"},
			nil, secrets{"token": "<token>"}),
		"flux-1-8": example("flux-1-8", "Flux with InfluxDB 1.8 database credentials", localURL,
			config{"product": "InfluxDB OSS 1.x", "version": "Flux", "organization": "organization", "defaultBucket": "site/autogen"},
			nil, secrets{"token": "<user>:<password>"}),
		"sql": example("sql", "Flight SQL with a database token over TLS", tlsURL,
			config{"product": "InfluxDB OSS 3.x", "version": "SQL", "dbName": "site"}, nil, secrets{"token": "<api-token>"}),
		"sql-pdc": example("sql-pdc", "Flight SQL through a Grafana Cloud PDC agent using a private CA", "https://influxdb3:8443",
			config{"product": "InfluxDB OSS 3.x", "version": "SQL", "dbName": "site", "enableSecureSocksProxy": true,
				"secureSocksProxyUsername": "<private-network-id>", "tlsAuthWithCACert": true},
			nil, secrets{"token": "<api-token>", "tlsCACert": "<PEM CA certificate>"}),
		"sql-socks-proxy": example("sql-socks-proxy", "Flight SQL through a SOCKS5 proxy with username and password", "https://influxdb3:8443",
			config{"product": "InfluxDB OSS 3.x", "version": "SQL", "dbName": "site", "enableSecureSocksProxy": true,
				"secureSocksProxyUsername": "<proxy-user>"},
			nil, secrets{"token": "<api-token>", "secureSocksProxyPassword": "<proxy-password>"}),
		"sql-plaintext": example("sql-plaintext", "Flight SQL on a plaintext server with authentication disabled", "http://influxdb3:8181",
			config{"product": "InfluxDB OSS 3.x", "version": "SQL", "dbName": "fixtures", "insecureGrpc": true}, nil, nil),
	}
}

func example(name, summary, url string, jsonData, root map[string]any, secrets map[string]string) *spec3.Example {
	spec := map[string]any{"title": summary, "access": "proxy", "url": url, "jsonData": jsonData}
	for key, value := range root {
		spec[key] = value
	}
	value := map[string]any{
		"apiVersion": "influxdb.datasource.grafana.app/v0alpha1",
		"kind":       "DataSource",
		"metadata":   map[string]string{"name": name},
		"spec":       spec,
	}
	if len(secrets) > 0 {
		secure := map[string]map[string]string{}
		for key, secret := range secrets {
			secure[key] = map[string]string{"create": secret}
		}
		value["secure"] = secure
	}
	return &spec3.Example{ExampleProps: spec3.ExampleProps{Summary: summary, Value: value}}
}
