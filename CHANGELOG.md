# Changelog

## Unreleased

- Fix a connection freeze between Grafana and the plugin under heavy query load by updating google.golang.org/grpc to v1.85.0 ([grpc/grpc-go#9330](https://github.com/grpc/grpc-go/issues/9330))

## 13.1.10

- Fix security vulnerabilities (GHSA-cg2g-xh64-v4q7, GHSA-mrv2-3pqr-j5jr, CVE-2026-93749, CVE-2023-54404, CVE-2026-78669, CVE-2026-78660, CVE-2026-78659)

## 13.1.9

- Bump backend dependency

## 13.1.8

- Chore: Pin golangci-lint to 2.15.0 in development dependencies and CI/CD pipelines

## 13.1.7

- Fix security vulnerabilities (CVE-2026-102278, CVE-2026-102276, CVE-2026-102990)

## 13.1.6

- Correct interpolation and escaping across InfluxQL, Flux and SQL ([#59](https://github.com/grafana/grafana-influxdb-datasource/pull/59))
- Fix security vulnerabilities (CVE-2026-84445)

## 13.1.5

- Report query errors instead of empty results, quote dotted table names once and make system tables queryable ([#60](https://github.com/grafana/grafana-influxdb-datasource/pull/60))

## 13.1.4

- Forward OAuth Identity for InfluxQL queries ([#92](https://github.com/grafana/grafana-influxdb-datasource/pull/92))
- Remove dead frontend code ([#61](https://github.com/grafana/grafana-influxdb-datasource/pull/61))

## 13.1.3

- Update dependencies

## 13.1.2

- Bump go v1.26.7 and grafana-plugin-sdk-go v0.296.4 ([#72](https://github.com/grafana/grafana-influxdb-datasource/pull/72))

## 13.1.1

- Correct error source for InfluxQL query errors ([#77](https://github.com/grafana/grafana-influxdb-datasource/pull/77))

## 13.1.0

- Support annotation queries in SQL mode ([#58](https://github.com/grafana/grafana-influxdb-datasource/pull/58))
- Dependency updates ([#66](https://github.com/grafana/grafana-influxdb-datasource/pull/66), [#68](https://github.com/grafana/grafana-influxdb-datasource/pull/68))

## 13.0.2

- Initial release of the InfluxDB data source as an external data source.
