# API Reference

MiniObserv exposes a JSON REST API over HTTP/HTTPS. All `/api/v1/*` endpoints require JWT authentication.

---

## Authentication

MiniObserv uses **HS256 JWTs** signed with the `AGENT_TOKEN` secret shared by the agent and the server.

The agent mints a new 24-hour token on startup. To call the API manually (e.g. with curl), mint a token yourself using any JWT HS256 library or the `jwt-cli` tool:

```bash
# Install jwt-cli once
npm install -g jwt-cli

# Mint a token valid for 24 hours
TOKEN=$(jwt sign --secret "YOUR_SECRET_HERE" --alg HS256 \
  '{"iss":"miniobserv-agent","exp":'$(( $(date +%s) + 86400 ))'}'  )

echo $TOKEN
```

Pass the token in every request via the `Authorization` header:

```
Authorization: Bearer <token>
```

**Token requirements:**
- Algorithm: HS256
- Issuer (`iss`): `miniobserv-agent`
- Expiry (`exp`): must be in the future
- Secret: same value as `AGENT_TOKEN` on the server (min 16 chars)

---

## POST /api/v1/metrics

Ingest a batch of metric data points. This is the endpoint the agent pushes to automatically.

### Request

```
POST /api/v1/metrics
Authorization: Bearer <token>
Content-Type: application/json
```

**Body schema:**

```json
{
  "host": "string (required)",
  "metrics": [
    {
      "time":   "string (RFC3339, required)",
      "host":   "string (required, should match top-level host)",
      "name":   "string (required, see Metric Names Reference)",
      "value":  "number (required)",
      "labels": {"key": "value"}
    }
  ]
}
```

**Constraints:**
- Maximum batch size: **1000 points per request**
- `time` must be a valid RFC3339 timestamp
- `host` at both the top level and per-metric level must be non-empty

### Example

```bash
TOKEN=$(jwt sign --secret "YOUR_SECRET_HERE" --alg HS256 \
  '{"iss":"miniobserv-agent","exp":'$(( $(date +%s) + 86400 ))'}'  )

curl -s -X POST http://localhost:8080/api/v1/metrics \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "host": "web-01",
    "metrics": [
      {
        "time":   "2026-06-05T10:00:00Z",
        "host":   "web-01",
        "name":   "cpu.usage_pct",
        "value":  42.5,
        "labels": {"core": "total"}
      },
      {
        "time":   "2026-06-05T10:00:00Z",
        "host":   "web-01",
        "name":   "mem.used_pct",
        "value":  67.1,
        "labels": {}
      }
    ]
  }'
```

### Responses

| Status | Body | Meaning |
|--------|------|---------|
| `202 Accepted` | `{"ingested": <n>}` | All points stored. |
| `400 Bad Request` | `{"error": "..."}` | Malformed JSON, missing required fields, or batch too large. |
| `401 Unauthorized` | `{"error": "unauthorized"}` | Missing, expired, or invalid JWT. |
| `500 Internal Server Error` | `{"error": "..."}` | Storage failure. |

```json
// 202
{"ingested": 2}

// 400
{"error": "missing host"}

// 401
{"error": "unauthorized"}
```

---

## GET /api/v1/metrics/query

Query time-bucketed metric series for a single host and metric name.

### Request

```
GET /api/v1/metrics/query
Authorization: Bearer <token>
```

**Query parameters:**

| Parameter | Required | Description |
|-----------|----------|-------------|
| `host` | yes | Hostname label. Must match the value used when ingesting. |
| `name` | yes | Metric name. See [Metric Names Reference](#metric-names-reference). |
| `from` | yes | Start of query window. RFC3339 format. |
| `to` | yes | End of query window. RFC3339 format. |
| `bucket` | no | Time bucket size. One of: `1m`, `5m`, `15m`, `1h`, `1d`. Default: `1m`. |
| `agg` | no | Aggregation function. One of: `avg`, `max`, `min`. Default: `avg`. |

**Constraints:**
- Maximum time range: **30 days** (`to` − `from` ≤ 30 days)
- `from` must be before `to`

### Examples

**Last hour of CPU usage, averaged per minute:**

```bash
TOKEN=$(jwt sign --secret "YOUR_SECRET_HERE" --alg HS256 \
  '{"iss":"miniobserv-agent","exp":'$(( $(date +%s) + 86400 ))'}'  )

curl -s -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/api/v1/metrics/query?host=web-01&name=cpu.usage_pct&from=2026-06-05T09:00:00Z&to=2026-06-05T10:00:00Z" \
  | jq .
```

**Peak memory usage per 5-minute bucket over the last 24 hours:**

```bash
curl -s -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/api/v1/metrics/query?host=web-01&name=mem.used_pct&from=2026-06-04T10:00:00Z&to=2026-06-05T10:00:00Z&bucket=5m&agg=max" \
  | jq .
```

**Minimum disk usage per hour over the last week:**

```bash
curl -s -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/api/v1/metrics/query?host=web-01&name=disk.used_pct&from=2026-05-29T00:00:00Z&to=2026-06-05T00:00:00Z&bucket=1h&agg=min" \
  | jq .
```

### Response

```json
{
  "host":   "web-01",
  "name":   "cpu.usage_pct",
  "bucket": "1m",
  "agg":    "avg",
  "points": [
    {"time": "2026-06-05T09:00:00Z", "value": 14.2},
    {"time": "2026-06-05T09:01:00Z", "value": 18.7},
    {"time": "2026-06-05T09:02:00Z", "value": 22.1}
  ]
}
```

If no data exists for the requested window, `points` is an empty array (not null):

```json
{"host":"web-01","name":"cpu.usage_pct","bucket":"1m","agg":"avg","points":[]}
```

### Responses

| Status | Body | Meaning |
|--------|------|---------|
| `200 OK` | See above | Query executed. `points` may be empty. |
| `400 Bad Request` | `{"error": "..."}` | Missing or invalid parameters. |
| `401 Unauthorized` | `{"error": "unauthorized"}` | Missing, expired, or invalid JWT. |

---

## GET /api/v1/hosts

Returns the current health status for all hosts known to the server. Public — no authentication required.

The server tracks the last time each host sent metrics. Status is derived from how long ago that was:

| Status | Condition |
|--------|-----------|
| `ok` | Host reported metrics within `HOST_STALE_AFTER` (default 20s) |
| `stale` | Silent for longer than `HOST_STALE_AFTER`, but less than `HOST_DOWN_AFTER` |
| `down` | Silent for longer than `HOST_DOWN_AFTER` (default 50s); a `host.down` webhook has fired |

### Request

```
GET /api/v1/hosts
```

### Example

```bash
curl -s http://localhost:8080/api/v1/hosts | jq .
```

### Response

```json
{
  "hosts": [
    {
      "host": "web-01",
      "status": "ok",
      "last_seen": "2026-06-05T16:42:23Z"
    },
    {
      "host": "web-02",
      "status": "stale",
      "last_seen": "2026-06-05T16:41:58Z"
    },
    {
      "host": "api-01",
      "status": "down",
      "last_seen": "2026-06-05T16:40:10Z"
    }
  ]
}
```

`hosts` is an empty array if no agents have reported yet.

### Responses

| Status | Body | Meaning |
|--------|------|---------|
| `200 OK` | See above | Status returned for all known hosts. |

### Related env vars

| Variable | Default | Description |
|----------|---------|-------------|
| `HOST_STALE_AFTER` | `20s` | Duration after which a silent host is considered stale. Accepts Go duration strings. |
| `HOST_DOWN_AFTER` | `50s` | Duration after which a silent host is considered down and a `host.down` webhook fires. |

---

## GET /healthz

Liveness probe. Returns `200 OK` with the body `ok` if the server process is running. Does not check the database.

```bash
curl http://localhost:8080/healthz
# → 200 OK
# → ok
```

Use this as a container liveness probe. If it fails, the server process itself has crashed.

---

## GET /readyz

Readiness probe. Returns `200 OK` with the body `ok` if the server is ready to handle traffic, including a successful database ping. Returns `503 Service Unavailable` if the database is not reachable.

```bash
curl -i http://localhost:8080/readyz
# → HTTP/1.1 200 OK
# → ok

# If the DB is down:
# → HTTP/1.1 503 Service Unavailable
```

Use this as a Kubernetes readiness probe:

```yaml
readinessProbe:
  httpGet:
    path: /readyz
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 5
```

---

## Error Responses

All errors follow the same JSON envelope:

```json
{"error": "human-readable description"}
```

| Status | Meaning | Common causes |
|--------|---------|---------------|
| `400 Bad Request` | Client sent invalid data | Malformed JSON, missing required field, invalid parameter value, batch too large, time range too wide |
| `401 Unauthorized` | Authentication failed | Missing `Authorization` header, expired JWT, wrong secret, wrong issuer |
| `500 Internal Server Error` | Server-side failure | Database write error, unexpected panic |
| `503 Service Unavailable` | Server not ready | Database unreachable (readyz only) |

---

## Metric Names Reference

The following metric names are collected by the agent and accepted by the ingestion endpoint.

| Name | Labels | Unit | Description |
|------|--------|------|-------------|
| `cpu.usage_pct` | `core=total\|0\|1\|…` | % | CPU usage. `core=total` is the aggregate across all cores. Per-core values use the core index. |
| `mem.used_pct` | — | % | Memory used as a percentage of total. |
| `mem.used_bytes` | — | bytes | Memory currently in use. |
| `mem.total_bytes` | — | bytes | Total installed memory. |
| `disk.used_pct` | `mount=/` | % | Disk used percentage for the given mount point. |
| `disk.used_bytes` | `mount=/` | bytes | Disk space in use for the given mount point. |
| `disk.total_bytes` | `mount=/` | bytes | Total disk capacity for the given mount point. |
| `net.bytes_in` | `iface=eth0` | bytes | Network bytes received since the previous collection tick (delta). |
| `net.bytes_out` | `iface=eth0` | bytes | Network bytes sent since the previous collection tick (delta). |

> **Delta semantics for `net.*`:** the agent records cumulative counters on startup and emits the
> difference each tick. No `net.*` metrics are emitted on the first collection tick.

### Label examples

When querying, the `host` and `name` parameters identify the series. Labels are stored with the data but are not query parameters — the TimescaleDB hypertable aggregates over all label values for a given `(host, name)` pair.

---

## Rate Limits & Limits

| Limit | Value |
|-------|-------|
| Max points per ingestion batch | 1000 |
| Max query time range | 30 days |
| JWT lifetime (agent-minted) | 24 hours |
| Min `AGENT_TOKEN` length | 16 characters |
| `COLLECT_INTERVAL` range | 1s – 300s |
| `REQUEST_TIMEOUT` range | 1s – 120s |

---

## Integration Examples

### Push a custom metric from a shell script

You can push any metric that matches the schema — including metrics not collected by the built-in agent. Use this to integrate custom application metrics.

```bash
TOKEN=$(jwt sign --secret "YOUR_SECRET_HERE" --alg HS256 \
  '{"iss":"miniobserv-agent","exp":'$(( $(date +%s) + 86400 ))'}'  )

NOW=$(date -u +%Y-%m-%dT%H:%M:%SZ)

curl -s -X POST http://localhost:8080/api/v1/metrics \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"host\": \"deploy-runner\",
    \"metrics\": [
      {
        \"time\":   \"$NOW\",
        \"host\":   \"deploy-runner\",
        \"name\":   \"cpu.usage_pct\",
        \"value\":  $(awk 'NR==1{print 100-$NF}' <(top -bn1 | grep "Cpu(s)")),
        \"labels\": {\"core\": \"total\"}
      }
    ]
  }"
```

### Push metrics from Go

```go
import (
    "bytes"
    "encoding/json"
    "net/http"
    "time"
)

type Metric struct {
    Time   time.Time         `json:"time"`
    Host   string            `json:"host"`
    Name   string            `json:"name"`
    Value  float64           `json:"value"`
    Labels map[string]string `json:"labels"`
}

type Batch struct {
    Host    string   `json:"host"`
    Metrics []Metric `json:"metrics"`
}

func pushMetric(serverURL, token string, m Metric) error {
    batch := Batch{Host: m.Host, Metrics: []Metric{m}}
    body, _ := json.Marshal(batch)

    req, _ := http.NewRequest("POST", serverURL+"/api/v1/metrics", bytes.NewReader(body))
    req.Header.Set("Authorization", "Bearer "+token)
    req.Header.Set("Content-Type", "application/json")

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    return nil
}
```

### Query metrics from Python

```python
import requests
from datetime import datetime, timedelta, timezone

TOKEN = "your-jwt-token-here"
BASE_URL = "http://localhost:8080"

now = datetime.now(timezone.utc)
one_hour_ago = now - timedelta(hours=1)

resp = requests.get(
    f"{BASE_URL}/api/v1/metrics/query",
    headers={"Authorization": f"Bearer {TOKEN}"},
    params={
        "host": "web-01",
        "name": "cpu.usage_pct",
        "from": one_hour_ago.strftime("%Y-%m-%dT%H:%M:%SZ"),
        "to": now.strftime("%Y-%m-%dT%H:%M:%SZ"),
        "bucket": "5m",
        "agg": "avg",
    },
)

data = resp.json()
for point in data["points"]:
    print(f"{point['time']}  CPU avg: {point['value']:.1f}%")
```
