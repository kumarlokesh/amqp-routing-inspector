#!/bin/bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

TIMEOUT_SECS_INSPECTOR=${TIMEOUT_SECS_INSPECTOR:-30}
TIMEOUT_SECS_DOCKER_STEP=${TIMEOUT_SECS_DOCKER_STEP:-20}
TIMEOUT_SECS_READY=${TIMEOUT_SECS_READY:-15}
TIMEOUT_SECS_DOCKER_BUILD=${TIMEOUT_SECS_DOCKER_BUILD:-300}
TIMEOUT_SECS_CONSUMER=${TIMEOUT_SECS_CONSUMER:-20}
SKIP_BUILD=${SKIP_BUILD:-0}
CLEAN_ONEOFF=${CLEAN_ONEOFF:-1}

PROJECT_NAME=${PROJECT_NAME:-${COMPOSE_PROJECT_NAME:-$(basename "${ROOT_DIR}")}}

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
if [ "${SKIP_BUILD}" = "1" ]; then
  echo "[1/6] SKIP_BUILD=1 (skipping docker compose build inspector)"
  echo "[1/6] NOTE: if you've changed Go code, you likely need to rebuild the image (unset SKIP_BUILD)"
else
  run_step "[1/6] docker compose build inspector" "${TIMEOUT_SECS_DOCKER_BUILD}" docker compose build inspector
fi

echo "[2/6] Enable tracing (required for firehose)"
run_step "[2/6] rabbitmqctl trace_on" "${TIMEOUT_SECS_DOCKER_STEP}" docker compose exec -T rabbitmq rabbitmqctl trace_on -p /

consume_once() {
  local queue="$1"
  local timeout_secs="$2"

  if ! command -v go >/dev/null 2>&1; then
    echo "go is required for deliver smoke script"
    return 2
  fi

  local tmpdir
  tmpdir=$(mktemp -d)
  local file="${tmpdir}/consume_once.go"

  cat >"${file}" <<'EOF'
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	amqpURL := envOrDefault("AMQP_URL", "amqp://guest:guest@localhost:5672/")
	queue := ""
	timeout := 10 * time.Second

	flag.StringVar(&amqpURL, "amqp-url", amqpURL, "AMQP URL")
	flag.StringVar(&queue, "queue", queue, "Queue name")
	flag.DurationVar(&timeout, "timeout", timeout, "Timeout")
	flag.Parse()

	queue = strings.TrimSpace(queue)
	if queue == "" {
		_, _ = fmt.Fprintln(os.Stderr, "queue is required")
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "dial rabbitmq: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = conn.Close() }()

	ch, err := conn.Channel()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "open channel: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = ch.Close() }()

	deliveries, err := ch.Consume(queue, "", true, true, false, false, nil)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "consume queue %q: %v\n", queue, err)
		os.Exit(1)
	}

	select {
	case <-ctx.Done():
		_, _ = fmt.Fprintf(os.Stderr, "timed out waiting for delivery from queue %q\n", queue)
		os.Exit(1)
	case _, ok := <-deliveries:
		if !ok {
			_, _ = fmt.Fprintf(os.Stderr, "delivery channel closed for queue %q\n", queue)
			os.Exit(1)
		}
		return
	}
}

func envOrDefault(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return fallback
}
EOF

  AMQP_URL=${AMQP_URL:-amqp://guest:guest@localhost:5672/} go run "${file}" --queue "${queue}" --timeout "${timeout_secs}s"
  rm -rf "${tmpdir}"
}

run_mode () {
  mode="$1"
  suffix="$(date +%s)_$RANDOM"
  ex="smoke.deliver.${mode}.${suffix}"
  q="smoke.deliver.${mode}.q.${suffix}"
  rk="smoke.deliver.${mode}.rk.${suffix}"
  traceq="smoke.deliver.${mode}.trace.${suffix}"
  payload="hello-deliver-${mode}-${suffix}"
  message_id="msg-deliver-${mode}-${suffix}"

  created_exchanges+=("$ex")
  created_queues+=("$q")
  created_queues+=("$traceq")

  echo
  echo "===== MODE=${mode} (deliver) ====="
  echo "[A] Declare exchange/queue/binding"
  run_step "[A1] declare exchange $ex" "${TIMEOUT_SECS_DOCKER_STEP}" docker compose exec -T rabbitmq rabbitmqadmin -q declare exchange name="$ex" type=topic durable=false auto_delete=true
  run_step "[A2] declare queue $q" "${TIMEOUT_SECS_DOCKER_STEP}" docker compose exec -T rabbitmq rabbitmqadmin -q declare queue name="$q" durable=false auto_delete=true
  run_step "[A3] bind queue" "${TIMEOUT_SECS_DOCKER_STEP}" docker compose exec -T rabbitmq rabbitmqadmin -q declare binding source="$ex" destination_type=queue destination="$q" routing_key="$rk"
  run_step "[A4] declare trace queue $traceq" "${TIMEOUT_SECS_DOCKER_STEP}" docker compose exec -T rabbitmq rabbitmqadmin -q declare queue name="$traceq" durable=false auto_delete=true
  run_step "[A5] bind trace queue" "${TIMEOUT_SECS_DOCKER_STEP}" docker compose exec -T rabbitmq rabbitmqadmin -q declare binding source=amq.rabbitmq.trace destination_type=queue destination="$traceq" routing_key="#"

  out="$(mktemp)"
  err="$(mktemp)"

  echo "[B] Start inspector (wait for 1 matched deliver event) in background"
  (
    docker compose run --rm inspector \
      --config /app/configs/config.docker.yaml \
      --queue-name "" \
      --output "$mode" \
      --filter-exchange "$ex" \
      --max-events 2 \
      >"$out" 2>"$err"
  ) &
  inspector_pid=$!

  echo "[C] Wait for inspector readiness (up to ${TIMEOUT_SECS_READY}s)"
  ready=0
  for i in $(seq 1 "${TIMEOUT_SECS_READY}"); do
    if grep -q "consuming firehose events" "$err" 2>/dev/null; then
      ready=1
      break
    fi
    if ! kill -0 "$inspector_pid" 2>/dev/null; then
      break
    fi
    sleep 1
  done
  if ! kill -0 "$inspector_pid" 2>/dev/null; then
    echo "[C] FAIL: inspector exited before publish"
    echo "=== ${mode} STDOUT ==="
    cat "$out"
    echo "=== ${mode} STDERR ==="
    cat "$err"
    exit 1
  fi
  if (( ready == 0 )); then
    echo "[C] WARN: inspector did not report readiness before publish"
  fi

  echo "[D] Start consumer (host AMQP)"
  consume_once "$q" "${TIMEOUT_SECS_CONSUMER}" &
  consumer_pid=$!

  echo "[E] Publish one message"
  run_step "[E1] publish message_id=$message_id" "${TIMEOUT_SECS_DOCKER_STEP}" docker compose exec -T rabbitmq rabbitmqadmin -q publish exchange="$ex" routing_key="$rk" payload="$payload" properties="{\"message_id\":\"$message_id\"}"

  echo "[F] Wait for consumer to receive message (up to ${TIMEOUT_SECS_CONSUMER}s)"
  waited=0
  while kill -0 "$consumer_pid" 2>/dev/null; do
    if (( waited >= TIMEOUT_SECS_CONSUMER )); then
      echo "[F] TIMEOUT waiting for consumer"
      kill "$consumer_pid" 2>/dev/null || true
      wait "$consumer_pid" 2>/dev/null || true
      break
    fi
    sleep 1
    waited=$((waited + 1))
  done
  wait "$consumer_pid" || true

  echo "[G] Wait up to ${TIMEOUT_SECS_INSPECTOR}s for inspector to exit"
  for i in $(seq 1 "${TIMEOUT_SECS_INSPECTOR}"); do
    if ! kill -0 "$inspector_pid" 2>/dev/null; then break; fi
    if (( i == 1 || i % 5 == 0 )); then
      echo "[G] ...still waiting (${i}s/${TIMEOUT_SECS_INSPECTOR}s)"
    fi
    sleep 1
  done
  if kill -0 "$inspector_pid" 2>/dev/null; then
    echo "[H] TIMEOUT: inspector still running -> killing"
    echo "[H] dumping trace queue ($traceq) to help diagnose deliver notifications"
    docker compose exec -T rabbitmq rabbitmqadmin -q get queue="$traceq" count=20 || true
    kill "$inspector_pid" || true
    cleanup_oneoff_inspector
  fi
  wait "$inspector_pid" || true

  echo "=== ${mode} STDOUT ==="
  cat "$out"
  echo "=== ${mode} STDERR ==="
  cat "$err"

  if ! grep -q "$ex" "$out"; then
    echo "[I] FAIL: expected output to contain exchange $ex"
    exit 1
  fi
  if ! grep -q "$q" "$out"; then
    echo "[I] FAIL: expected output to contain queue $q"
    exit 1
  fi

  rm -f "$out" "$err"
}

run_mode cli
run_mode json
run_mode dot

echo
echo "[6/6] Done"
