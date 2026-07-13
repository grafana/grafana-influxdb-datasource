# Fixture data

Deterministic InfluxDB line-protocol datasets loaded into every InfluxDB
container by the `*-loader` services in [docker-compose.yaml](../../docker-compose.yaml).
The same files are written to all three major versions (1.x, 2.x, 3.x) under
database/bucket `fixtures`.

| File | Measurement | Series |
| --- | --- | --- |
| `sensor.lp` | `sensor` | IoT sensor readings from 5 devices (temperature, humidity, pressure, CO₂) |
| `httplogs.lp` | `httplogs` | Structured HTTP access logs with latency, status codes, and trace context |
| `infra.lp` | `infra` | Host metrics (CPU, memory, disk, network) for 6 hosts |

## Time window

All points fall inside a fixed window. Tests asserting on fixture data must
query this range:

```text
from: 2026-06-01T00:00:00.000Z
to:   2026-06-01T04:00:00.000Z
```

Timestamps are nanosecond precision, one tick per minute, 240 ticks per
measurement.

## Regenerating

Generated with [time-series-datagen](https://github.com/adamyeats/time-series-datagen)
using a fixed seed, so output is reproducible:

```sh
time-series-datagen \
  -type sensor,httplogs,infra \
  -count 240 \
  -start 2026-06-01T00:00:00Z \
  -interval 1m \
  -seed 42 \
  -format lp \
  -output-dir tests/fixtures
```

The trace and span IDs in `httplogs.lp` are synthetic hex strings, not real
credentials. They are excluded from secret scanning in
[.trufflehogignore](../../.trufflehogignore), which the org-required
TruffleHog workflow appends to its central exclude list. The local file must
be named `.trufflehogignore` (one Go regex per line) — a `.trufflehog.yml`
is not read by that workflow.
