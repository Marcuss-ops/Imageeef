#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MASTER_URL="${MASTER_URL:-http://127.0.0.1:8000}"
IMAGE_TAG="${IMAGE_TAG:-velox-worker:test}"
WORKER_BASE_DIR="${WORKER_BASE_DIR:-$HOME/.velox/local-workers}"
WORK_DIR="${WORK_DIR:-/app/RemoteCodex}"

WORKERS=(
  "host_57_129_132_133"
  "host_149_56_131_97"
  "host_51_222_204_158"
)

usage() {
  cat <<EOF
Usage: $(basename "$0") <up|down|status>

Environment:
  MASTER_URL       Master URL to register against (default: $MASTER_URL)
  IMAGE_TAG        Worker image tag (default: $IMAGE_TAG)
  WORKER_BASE_DIR  Persistent config directory root (default: $WORKER_BASE_DIR)
  WORK_DIR         Worker work dir inside the container (default: $WORK_DIR)
EOF
}

write_config() {
  local worker_id="$1"
  local worker_dir="$WORKER_BASE_DIR/$worker_id"
  mkdir -p "$worker_dir"
  cat > "$worker_dir/worker_config.json" <<EOF
{
  "master_url": "$MASTER_URL",
  "worker_id": "$worker_id",
  "worker_name": "velox-worker",
  "work_dir": "$WORK_DIR",
  "log_level": "info",
  "bundle_version": "v1.0.6",
  "protocol_version": "2026-06-worker-v1",
  "engine_version": "v1.0.6",
  "max_active_jobs": 1,
  "command_poll_interval_secs": 30,
  "prometheus_port": 0,
  "health_port": 8081
}
EOF
}

start_worker() {
  local worker_id="$1"
  local worker_dir="$WORKER_BASE_DIR/$worker_id"
  local container_name="velox-worker-$worker_id"

  write_config "$worker_id"
  docker rm -f "$container_name" >/dev/null 2>&1 || true
  docker run -d \
    --name "$container_name" \
    --restart unless-stopped \
    --add-host host.docker.internal:host-gateway \
    -e VELOX_VIDEO_ENGINE_SRC=/app/RemoteCodex/native/video-engine-cpp \
    -v "$worker_dir:/tmp/velox-worker" \
    -v "$ROOT_DIR:/app" \
    "$IMAGE_TAG" \
    -config /tmp/velox-worker/worker_config.json \
    -master "$MASTER_URL" \
    -worker-id "$worker_id" \
    -work-dir "$WORK_DIR" >/dev/null
}

stop_worker() {
  local worker_id="$1"
  docker rm -f "velox-worker-$worker_id" >/dev/null 2>&1 || true
}

cmd="${1:-}"
case "$cmd" in
  up)
    for worker_id in "${WORKERS[@]}"; do
      start_worker "$worker_id"
    done
    ;;
  down)
    for worker_id in "${WORKERS[@]}"; do
      stop_worker "$worker_id"
    done
    ;;
  status)
    docker ps --format '{{.Names}} {{.Status}}' | rg '^velox-worker-host_' || true
    ;;
  *)
    usage
    exit 1
    ;;
esac
