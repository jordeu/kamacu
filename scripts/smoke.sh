#!/usr/bin/env bash
# End-to-end smoke test: proves single-binary serving (STOR-02) and
# restart persistence (STOR-01) over real HTTP against bin/kangent.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

ADDR="127.0.0.1:7402"
BASE="http://$ADDR"
SERVER_PID=""

fail() {
  echo "FAIL: $1" >&2
  exit 1
}

cleanup() {
  if [ -n "$SERVER_PID" ]; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  rm -rf "$WORK"
}

start_server() {
  ./bin/kangent --addr "$ADDR" --db "$WORK/k.db" &
  SERVER_PID=$!
  for _ in $(seq 1 50); do
    if curl -fsS "$BASE/api/healthz" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.1
  done
  fail "server did not answer /api/healthz within 5s"
}

stop_server() {
  kill "$SERVER_PID"
  wait "$SERVER_PID" 2>/dev/null || true
  SERVER_PID=""
}

# --- 1. Build ---------------------------------------------------------------
make build >/dev/null

# --- 2. Workspace: throwaway git repo + db dir ------------------------------
WORK="$(mktemp -d)"
trap cleanup EXIT
mkdir "$WORK/repo"
git -C "$WORK/repo" init -q

# --- 3. Start server ---------------------------------------------------------
start_server

# --- 4. Create project (default-name rule) -----------------------------------
RESP="$(curl -s -w '\n%{http_code}' -X POST "$BASE/api/projects" \
  -H 'Content-Type: application/json' \
  -d "{\"repo_path\":\"$WORK/repo\"}")"
CODE="$(echo "$RESP" | tail -1)"
BODY="$(echo "$RESP" | sed '$d')"
[ "$CODE" = "201" ] || fail "create project: expected 201, got $CODE ($BODY)"
echo "$BODY" | grep -q '"name":"repo"' || fail "create project: name did not default to \"repo\" ($BODY)"
PID="$(echo "$BODY" | grep -o '"id":[0-9]*' | head -1 | cut -d: -f2)"
[ -n "$PID" ] || fail "create project: could not extract project id ($BODY)"

# --- 5. Negative: invalid repo path → 400 ------------------------------------
CODE="$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/api/projects" \
  -H 'Content-Type: application/json' \
  -d '{"repo_path":"/nonexistent-zzz"}')"
[ "$CODE" = "400" ] || fail "invalid repo path: expected 400, got $CODE"

# --- 6. Create task -----------------------------------------------------------
RESP="$(curl -s -w '\n%{http_code}' -X POST "$BASE/api/projects/$PID/tasks" \
  -H 'Content-Type: application/json' \
  -d '{"title":"smoke task","description":"# md"}')"
CODE="$(echo "$RESP" | tail -1)"
BODY="$(echo "$RESP" | sed '$d')"
[ "$CODE" = "201" ] || fail "create task: expected 201, got $CODE ($BODY)"
TID="$(echo "$BODY" | grep -o '"id":[0-9]*' | head -1 | cut -d: -f2)"
[ -n "$TID" ] || fail "create task: could not extract task id ($BODY)"

# --- 7. Move task to in_review -------------------------------------------------
CODE="$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/api/tasks/$TID/move" \
  -H 'Content-Type: application/json' \
  -d '{"status":"in_review","after_id":null}')"
[ "$CODE" = "200" ] || fail "move task: expected 200, got $CODE"

# --- 8. Edit description --------------------------------------------------------
CODE="$(curl -s -o /dev/null -w '%{http_code}' -X PATCH "$BASE/api/tasks/$TID" \
  -H 'Content-Type: application/json' \
  -d '{"description":"updated"}')"
[ "$CODE" = "200" ] || fail "patch task: expected 200, got $CODE"

# --- 9. Restart: kill server, start fresh instance on the same db ---------------
stop_server
start_server

# --- 10. Assert persistence across restart --------------------------------------
BODY="$(curl -fsS "$BASE/api/projects")" || fail "list projects after restart"
echo "$BODY" | grep -q "\"id\":$PID" || fail "project $PID missing after restart ($BODY)"

BODY="$(curl -fsS "$BASE/api/tasks/$TID")" || fail "get task after restart"
echo "$BODY" | grep -q '"status":"in_review"' || fail "task status not in_review after restart ($BODY)"
echo "$BODY" | grep -q '"description":"updated"' || fail "task description not updated after restart ($BODY)"

# --- 11. SPA serving checks -------------------------------------------------------
curl -fsS "$BASE/" | grep -qi '<html' || fail "GET / did not return HTML"
curl -fsS "$BASE/projects/$PID/tasks/$TID" | grep -qi '<html' || fail "deep link did not return HTML"
CODE="$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/nope")"
[ "$CODE" = "404" ] || fail "GET /api/nope: expected 404, got $CODE"

# --- 12. Done ----------------------------------------------------------------------
echo "SMOKE OK"
