# Decouple Query Execution from Response Parsing

**Date:** 2026-07-13
**Branch:** `refactor/decouple-execution-parsing`, stacked on `feat/parallel-query-execution` (PR #62), which is stacked on `add-influxdb-version-matrix` (PR #57)
**Roadmap item:** #1 (priority: medium)
**Driver:** testability and maintainability only. This is a pure refactor with no behaviour change.

## Problem

Each query language currently fuses transport and response parsing in a single function body:

- **InfluxQL** (`pkg/influxdb/influxql/influxql.go`, `execute`): the HTTP round trip, tracing, parser selection (a `bool` threaded through the call chain), body parsing, and custom-metadata header stamping share one function.
- **Flux** (`pkg/influxdb/flux/executor.go`, `executeQuery`): interpolation, running the query, frame building (`readDataFrames`), max-points error rewriting, and frame-meta stamping share one function. `readDataFrames` takes the concrete `*api.QueryTableResult`, so frame building cannot be unit-tested without a live client.
- **FSQL** (`pkg/influxdb/fsql/fsql.go`, `Execute`): the Flight SQL gRPC calls and Arrow-to-frame conversion interleave, though the parse half already consumes a `recordReader` interface via `newQueryDataResponse`.

Changes to one half routinely force edits and re-review of the other, and the parse paths lack focused unit tests.

## Non-goals

- No performance work. The seams expose the types natural to each protocol, not streams designed for pipelining.
- No shared cross-language transport/parser abstraction in the root package. The three result types (HTTP JSON body, Flux table stream, Arrow Flight reader) have nothing useful in common.
- No changes to feature toggles, the `runQueries` fan-out, `newQueryExecutor`, health checks, or any public behaviour.

## Design

The PR #62 contract is fixed: each package keeps its `Executor` facade with `Execute(ctx context.Context, query backend.DataQuery) backend.DataResponse` and `Close() error`. Inside each package, `Execute` is reorganised into two stages, **run** and **parse**, with a named seam between them. Consumer-defined interfaces sit at each seam, following the Go style guide.

### InfluxQL

- Add a package-local parser type: `type responseParser func(io.ReadCloser, int, *models.Query) *backend.DataResponse`. Both existing parsers (`buffered.ResponseParse`, `querydata.ResponseParse`) already match this signature exactly.
- `Executor` gains a `parse responseParser` field. `NewExecutor` selects the parser once from the `influxqlStreamingParser` toggle it already reads, replacing the `streamingParserEnabled` bool currently threaded through `execute`.
- Split `execute` into:
  - `runQuery(ctx, request) (*http.Response, error)`: the HTTP round trip only.
  - `parseResponse(res *http.Response, query *models.Query) backend.DataResponse`: the tracing span around parsing, invoking the chosen parser, custom-metadata stamping (`readCustomMetadata`), and closing the body.
- Parser selection becomes a real strategy seam as a side effect.

### Flux

- Split `executeQuery` along the run/parse line:
  - Run: interpolation plus `runner.runQuery` (the existing `queryRunner` interface already isolates transport).
  - Parse: `readDataFrames` plus the max-points error rewriting and `ExecutedQueryString`/meta stamping, moved together so the parse side owns everything response-shaped.
- `readDataFrames` takes a new package-local `tableResult` interface (`Next() bool`, `TableChanged() bool`, `TableMetadata() *query.FluxTableMetadata`, `Record() *query.FluxRecord`, `Err() error`) instead of the concrete `*api.QueryTableResult`, making frame building unit-testable with a fake stream for the first time.

### FSQL

- Split `Execute` into:
  - `runQuery(ctx, sql) (reader, headers, error)`: `client.Execute`, the endpoint-count check, `DoGetWithHeaderExtraction`, and header extraction.
  - Parse: the existing `newQueryDataResponse(reader, query, headers)`, unchanged.

## Deviations found during implementation

- **Flux:** the `ExecutedQueryString` stamping stays in `executeQuery` rather than moving to the parse side, because it applies to the transport-error path too and moving it would change behaviour. The max-points error rewrite moved as planned.
- **InfluxQL:** the run stage is the inline `HTTPClient.Do` call in `Execute` rather than a named `runQuery` function, as it is a single line. The per-query "streaming parser enabled" log line moved to `NewExecutor`, so it now logs once per request instead of once per query.
- **FSQL:** `runQuery` returns `(*flightReader, *backend.DataResponse)` rather than `(reader, headers, error)`. The run stage owns its three distinct error mappings, so returning a plain error would have forced `Execute` to re-derive which mapping applied. Header extraction stays in `Execute` beside the reader lifecycle.
- **Bug found by the new tests:** the streaming InfluxQL parser (`converter.ReadInfluxQLStyleResult`) panicked with a nil-pointer dereference on any response whose top-level JSON opens with an `"error"` field before any `"results"` field, which InfluxDB emits for auth failures and unknown databases. The buffered parser handled the same body correctly. Fixed unconditionally in `pkg/influxdb/influxql/converter/converter.go`, in the same spirit as the FSQL batch-abandonment fix in PR #62.

## Error handling

Error mapping stays exactly where it is today. Transport errors keep their current `ErrorSource` and status mapping on the run side (`errorResponse` for FSQL, `http.Error` handling for Flux, downstream-source tagging for InfluxQL). Parse errors stay on the parse side. Every failure is still reported inside the returned `backend.DataResponse`, never as a panic, preserving the PR #62 contract that one failing query cannot affect its siblings.

## Testing

- The concurrency race tests, benchmarks, and the InfluxDB version-matrix integration tests from the stack must pass unchanged. They are the no-behaviour-change proof.
- New table-driven unit tests:
  - Flux: `readDataFrames` against a fake `tableResult`, covering the max-series and invalid-state branches that are currently unreachable in tests.
  - InfluxQL: `parseResponse` with canned `*http.Response` bodies, exercising both parser strategies and custom-metadata stamping.
  - FSQL: `runQuery` error paths (endpoint-count check, gRPC error mapping) with a fake client where one does not already exist.
- Tests use testify `require`, matching the repo convention.

## Delivery

- One PR on `refactor/decouple-execution-parsing`, targeting `feat/parallel-query-execution`. Retarget down the stack as parents merge (#57 first, then #62).
- One commit per language (InfluxQL, Flux, FSQL) plus a final commit for any shared test additions. Conventional Commits, lowercase subjects.
- Every commit and push requires explicit user sign-off (grafana-org rule).
- After each commit: `go build ./...` and the package tests pass.
