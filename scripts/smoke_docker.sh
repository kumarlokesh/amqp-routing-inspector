#!/bin/bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

CLEAN_ONEOFF=${CLEAN_ONEOFF:-1}
PROJECT_NAME=${PROJECT_NAME:-${COMPOSE_PROJECT_NAME:-$(basename "${ROOT_DIR}")}}

TIMEOUT_SECS_INSPECTOR=${TIMEOUT_SECS_INSPECTOR:-20}
TIMEOUT_SECS_DOCKER_STEP=${TIMEOUT_SECS_DOCKER_STEP:-15}
TIMEOUT_SECS_READY=${TIMEOUT_SECS_READY:-10}
TIMEOUT_SECS_DOCKER_BUILD=${TIMEOUT_SECS_DOCKER_BUILD:-300}
SKIP_BUILD=${SKIP_BUILD:-0}

created_exchanges=()
created_queues=()

cleanup_oneoff_inspector() {
  local ids
  ids=$(docker ps -aq \
    --filter "label=com.docker.compose.project=${PROJECT_NAME}" \
    --filter "label=com.docker.compose.service=inspector" \
    --filter "label=com.docker.compose.oneoff=True" 2>/dev/null || true)
  if [ -n "${ids}" ]; then
    docker rm -f ${ids} >/dev/null 2>&1 || true
  fi
}

run_step() {
  local desc="$1"
  local timeout="$2"
  shift 2

  echo "$desc"

  "$@" &
  local pid=$!
  local elapsed=0
  while kill -0 "$pid" 2>/dev/null; do
    if (( elapsed >= timeout )); then
      echo "[TIMEOUT] $desc"
      kill "$pid" 2>/dev/null || true
      wait "$pid" 2>/dev/null || true
      return 124
    fi
    sleep 1
    elapsed=$((elapsed + 1))
  done
  wait "$pid"
}

cleanup() {
  set +e
  if (( ${#created_queues[@]} > 0 )); then
    for q in "${created_queues[@]}"; do
      docker compose exec -T rabbitmq rabbitmqadmin -q delete queue name="$q" >/dev/null 2>&1 || true
    done
  fi
  if (( ${#created_exchanges[@]} > 0 )); then
    for ex in "${created_exchanges[@]}"; do
      docker compose exec -T rabbitmq rabbitmqadmin -q delete exchange name="$ex" >/dev/null 2>&1 || true
    done
  fi

  cleanup_oneoff_inspector
}

trap cleanup EXIT

if [ "${CLEAN_ONEOFF}" = "1" ]; then
  echo "[pre] cleaning one-off inspector containers for project ${PROJECT_NAME}"
  cleanup_oneoff_inspector
fi

run_step "[0/6] docker compose ps" "${TIMEOUT_SECS_DOCKER_STEP}" docker compose ps

echo "[1/6] Ensure inspector image exists (skip if already built)"
# If you already built before, this is fast/no-op; otherwise it builds once.
if [ "${SKIP_BUILD}" = "1" ]; then
  echo "[1/6] SKIP_BUILD=1 (skipping docker compose build inspector)"
else
  run_step "[1/6] docker compose build inspector" "${TIMEOUT_SECS_DOCKER_BUILD}" docker compose build inspector
fi

echo "[2/6] Enable tracing (required for firehose)"
run_step "[2/6] rabbitmqctl trace_on" "${TIMEOUT_SECS_DOCKER_STEP}" docker compose exec -T rabbitmq rabbitmqctl trace_on -p /

run_mode () {
  mode="$1"
  suffix="$(date +%s)_$RANDOM"
  ex="smoke.${mode}.${suffix}"
  q="smoke.${mode}.q.${suffix}"
  rk="smoke.${mode}.rk.${suffix}"
  payload="hello-${mode}-${suffix}"
  message_id="msg-${mode}-${suffix}"

  created_exchanges+=("$ex")
  created_queues+=("$q")

  echo
  echo "===== MODE=${mode} ====="
  echo "[A] Declare exchange/queue/binding"
  run_step "[A1] declare exchange $ex" "${TIMEOUT_SECS_DOCKER_STEP}" docker compose exec -T rabbitmq rabbitmqadmin -q declare exchange name="$ex" type=topic durable=false auto_delete=true
  run_step "[A2] declare queue $q" "${TIMEOUT_SECS_DOCKER_STEP}" docker compose exec -T rabbitmq rabbitmqadmin -q declare queue name="$q" durable=false auto_delete=true
  run_step "[A3] bind queue" "${TIMEOUT_SECS_DOCKER_STEP}" docker compose exec -T rabbitmq rabbitmqadmin -q declare binding source="$ex" destination_type=queue destination="$q" routing_key="$rk"

  out="$(mktemp)"
  err="$(mktemp)"

  echo "[B] Start inspector (wait for 1 matched event) in background"
  (
    docker compose run --rm inspector \
      --config /app/configs/config.docker.yaml \
      --queue-name "" \
      --output "$mode" \
      --filter-exchange "$ex" \
      --max-events 1 \
      >"$out" 2>"$err"
  ) &
  pid=$!

  echo "[C] Publish one message"
  echo "[C0] waiting for inspector readiness (up to ${TIMEOUT_SECS_READY}s)"
  ready=0
  for i in $(seq 1 "${TIMEOUT_SECS_READY}"); do
    if grep -q "unknown flag" "$err" 2>/dev/null; then
      break
    fi
    if grep -q "consuming firehose events" "$err" 2>/dev/null; then
      ready=1
      break
    fi
    if ! kill -0 "$pid" 2>/dev/null; then
      break
    fi
    sleep 1
  done
  if ! kill -0 "$pid" 2>/dev/null; then
    echo "[C0] FAIL: inspector exited before publish"
    echo "=== ${mode} STDOUT ==="
    cat "$out"
    echo "=== ${mode} STDERR ==="
    cat "$err"
    exit 1
  fi
  if (( ready == 0 )); then
    echo "[C0] WARN: inspector did not report readiness before publish"
  fi
  run_step "[C1] publish message_id=$message_id" "${TIMEOUT_SECS_DOCKER_STEP}" docker compose exec -T rabbitmq rabbitmqadmin -q publish exchange="$ex" routing_key="$rk" payload="$payload" properties="{\"message_id\":\"$message_id\"}"

  echo "[D] Wait up to ${TIMEOUT_SECS_INSPECTOR}s (hard timeout so we never hang)"
  for i in $(seq 1 "${TIMEOUT_SECS_INSPECTOR}"); do
    if ! kill -0 "$pid" 2>/dev/null; then break; fi
    if (( i == 1 || i % 5 == 0 )); then
      echo "[D] ...still waiting (${i}s/${TIMEOUT_SECS_INSPECTOR}s)"
    fi
    sleep 1
  done
  if kill -0 "$pid" 2>/dev/null; then
    echo "[E] TIMEOUT: inspector still running -> killing"
    kill "$pid" || true
    cleanup_oneoff_inspector
  fi
  wait "$pid" || true

  echo "=== ${mode} STDOUT ==="
  cat "$out"
  echo "=== ${mode} STDERR ==="
  cat "$err"

  if ! grep -q "$ex" "$out"; then
    echo "[F] FAIL: expected output to contain exchange $ex"
    exit 1
  fi

  rm -f "$out" "$err"
}

run_mode cli
run_mode json
run_mode dot

echo
echo "[6/6] Done"
