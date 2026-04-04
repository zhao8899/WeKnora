#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://127.0.0.1:18080/api/v1}"
API_KEY="${API_KEY:-}"
KB_ID="${KB_ID:-}"

if [[ -z "${API_KEY}" ]]; then
  echo "API_KEY is required" >&2
  exit 1
fi

if [[ -z "${KB_ID}" ]]; then
  echo "KB_ID is required" >&2
  exit 1
fi

health_out="$(mktemp)"
diagnostics_out="$(mktemp)"
cleanup() {
  rm -rf "${tmpdir:-}" "${health_out}" "${diagnostics_out}"
}
trap cleanup EXIT

if ! curl -fsS "${BASE_URL%/api/v1}/health" > "${health_out}" 2>/dev/null; then
  echo "server health check failed: ${BASE_URL%/api/v1}/health" >&2
  exit 1
fi

if ! curl -fsS -H "X-API-Key: ${API_KEY}" "${BASE_URL}/system/diagnostics" > "${diagnostics_out}" 2>/dev/null; then
  echo "system diagnostics check failed: ${BASE_URL}/system/diagnostics" >&2
  exit 1
fi

tmpdir="$(mktemp -d)"

create_session() {
  local title="$1"
  curl -sS -X POST "${BASE_URL}/sessions" \
    -H "Content-Type: application/json" \
    -H "X-API-Key: ${API_KEY}" \
    -d "{\"title\":\"${title}\"}" | python3 -c 'import sys,json; print(json.load(sys.stdin)["data"]["id"])'
}

run_sse() {
  local session_id="$1"
  local payload="$2"
  curl -sS -N "${BASE_URL}/knowledge-chat/${session_id}" \
    -H "Content-Type: application/json" \
    -H "X-API-Key: ${API_KEY}" \
    -d "${payload}"
}

assert_contains() {
  local file="$1"
  local pattern="$2"
  if ! grep -q -- "${pattern}" "${file}"; then
    echo "expected pattern not found: ${pattern}" >&2
    echo "--- output ---" >&2
    cat "${file}" >&2
    exit 1
  fi
}

chat_session="$(create_session "smoke-chat")"
rag_fast_session="$(create_session "smoke-rag-fast")"
rag_deep_session="$(create_session "smoke-rag-deep")"

chat_out="${tmpdir}/chat.sse"
rag_fast_out="${tmpdir}/rag_fast.sse"
rag_deep_out="${tmpdir}/rag_deep.sse"

run_sse "${chat_session}" '{"query":"请只回复 chat-mode-ok","mode":"chat","channel":"api"}' > "${chat_out}"
run_sse "${rag_fast_session}" "{\"query\":\"道成是什么？请简短回答。\",\"mode\":\"rag_fast\",\"knowledge_base_ids\":[\"${KB_ID}\"],\"channel\":\"api\"}" > "${rag_fast_out}"
run_sse "${rag_deep_session}" "{\"query\":\"总结一下道成知识库里的核心观点。\",\"mode\":\"rag_deep\",\"knowledge_base_ids\":[\"${KB_ID}\"],\"channel\":\"api\"}" > "${rag_deep_out}"

assert_contains "${chat_out}" '"response_type":"answer"'
assert_contains "${chat_out}" 'chat-mode-ok'

assert_contains "${health_out}" '"status":"ok"'
assert_contains "${health_out}" '"db"'
assert_contains "${health_out}" '"stream_manager"'

assert_contains "${diagnostics_out}" '"code":0'
assert_contains "${diagnostics_out}" '"docreader"'
assert_contains "${diagnostics_out}" '"retrieval"'
assert_contains "${diagnostics_out}" '"object_store"'

assert_contains "${rag_fast_out}" '"response_type":"references"'
assert_contains "${rag_fast_out}" '"response_type":"complete"'

assert_contains "${rag_deep_out}" '"response_type":"references"'
assert_contains "${rag_deep_out}" '"response_type":"complete"'

echo "health: ok"
echo "diagnostics: ok"
echo "chat: ok"
echo "rag_fast: ok"
echo "rag_deep: ok"
