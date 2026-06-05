#!/usr/bin/env bash
# MiniObserv full API lifecycle demo
# Uses: bash, curl, python3 (stdlib only — no extra packages)
#
# Usage:
#   export MINIOBSERV_URL=http://localhost:8080
#   export AGENT_TOKEN=your-secret-here-min-16-chars
#   bash run.sh

set -euo pipefail

MINIOBSERV_URL="${MINIOBSERV_URL:?MINIOBSERV_URL must be set}"
AGENT_TOKEN="${AGENT_TOKEN:?AGENT_TOKEN must be set}"

# ── colours ──────────────────────────────────────────────────────────────────
GREEN='\033[0;32m'; BLUE='\033[0;34m'; YELLOW='\033[1;33m'; RESET='\033[0m'
step() { echo -e "\n${BLUE}▶ $*${RESET}"; }
ok()   { echo -e "${GREEN}✓ $*${RESET}"; }

# ─────────────────────────────────────────────────────────────────────────────
# STEP 1 — Health check
# ─────────────────────────────────────────────────────────────────────────────
step "Step 1 — Liveness check (/healthz)"
curl -sf "${MINIOBSERV_URL}/healthz" | python3 -m json.tool
ok "Server is alive"

# ─────────────────────────────────────────────────────────────────────────────
# STEP 2 — Mint a HS256 JWT (Python stdlib, no PyJWT needed)
# The token is signed with AGENT_TOKEN and expires in 24 hours.
# ─────────────────────────────────────────────────────────────────────────────
step "Step 2 — Minting HS256 JWT via Python hmac"
TOKEN=$(python3 - <<'PYEOF'
import base64, hmac, hashlib, json, time, os, sys

secret = os.environ["AGENT_TOKEN"].encode()

def b64url(data: bytes) -> str:
    return base64.urlsafe_b64encode(data).rstrip(b"=").decode()

now = int(time.time())
header  = b64url(json.dumps({"alg": "HS256", "typ": "JWT"}).encode())
payload = b64url(json.dumps({"iss": "miniobserv-agent", "iat": now, "exp": now + 86400}).encode())
signing_input = f"{header}.{payload}".encode()
sig = b64url(hmac.new(secret, signing_input, hashlib.sha256).digest())
print(f"{header}.{payload}.{sig}")
PYEOF
)
echo -e "${YELLOW}JWT: ${TOKEN:0:60}...${RESET}"
ok "Token minted"

# ─────────────────────────────────────────────────────────────────────────────
# STEP 3 — Push a metric batch
# Sends 5 different metric types for host "demo-host".
# ─────────────────────────────────────────────────────────────────────────────
step "Step 3 — Pushing metric batch (5 metrics)"
NOW=$(python3 -c "from datetime import datetime, timezone; print(datetime.now(timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ'))")

RESPONSE=$(curl -sf -X POST "${MINIOBSERV_URL}/api/v1/metrics" \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{
    \"host\": \"demo-host\",
    \"metrics\": [
      {\"time\": \"${NOW}\", \"host\": \"demo-host\", \"name\": \"cpu.usage_pct\",    \"value\": 42.5},
      {\"time\": \"${NOW}\", \"host\": \"demo-host\", \"name\": \"mem.used_pct\",     \"value\": 67.3},
      {\"time\": \"${NOW}\", \"host\": \"demo-host\", \"name\": \"mem.used_bytes\",   \"value\": 4294967296},
      {\"time\": \"${NOW}\", \"host\": \"demo-host\", \"name\": \"disk.used_pct\",    \"value\": 55.1},
      {\"time\": \"${NOW}\", \"host\": \"demo-host\", \"name\": \"net.bytes_in\",     \"value\": 1048576}
    ]
  }")

echo "$RESPONSE" | python3 -m json.tool
ok "Batch ingested"

# ─────────────────────────────────────────────────────────────────────────────
# STEP 4 — Query metrics
# Queries cpu.usage_pct for the last 5 minutes, 1-minute buckets, average.
# ─────────────────────────────────────────────────────────────────────────────
step "Step 4 — Querying cpu.usage_pct (last 5 minutes, 1m buckets, avg)"
FROM=$(python3 -c "from datetime import datetime, timezone, timedelta; print((datetime.now(timezone.utc) - timedelta(minutes=5)).strftime('%Y-%m-%dT%H:%M:%SZ'))")
TO=$(python3 -c "from datetime import datetime, timezone; print(datetime.now(timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ'))")

QUERY_RESP=$(curl -sf \
  -H "Authorization: Bearer ${TOKEN}" \
  "${MINIOBSERV_URL}/api/v1/metrics/query?host=demo-host&name=cpu.usage_pct&from=${FROM}&to=${TO}&bucket=1m&agg=avg")

echo "$QUERY_RESP" | python3 -m json.tool
ok "Query returned"

# ─────────────────────────────────────────────────────────────────────────────
# STEP 5 — Pretty-print the result as a table
# ─────────────────────────────────────────────────────────────────────────────
step "Step 5 — Pretty-print query result as table"
echo "$QUERY_RESP" | python3 - <<'PYEOF'
import json, sys

data = json.load(sys.stdin)
print(f"\n  Host  : {data['host']}")
print(f"  Metric: {data['name']}")
print(f"  Bucket: {data['bucket']}   Agg: {data['agg']}")
print()
print(f"  {'Time':<30}  {'Value':>10}")
print(f"  {'-'*30}  {'-'*10}")
for pt in data.get("points", []):
    print(f"  {pt['time']:<30}  {pt['value']:>10.2f}")
if not data.get("points"):
    print("  (no points — data may need a moment to aggregate)")
PYEOF

ok "Done! Full API lifecycle completed."
