#!/usr/bin/env bash
set -euo pipefail

APP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE_FILE="$APP_DIR/docker-compose.yml"
ENV_FILE="$APP_DIR/.env"
ENV_EXAMPLE="$APP_DIR/.env.example"

log() {
  echo "[aweh-deploy] $*"
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Missing required command: $1" >&2
    exit 1
  fi
}

check_prereqs() {
  require_cmd docker
  if ! docker compose version >/dev/null 2>&1; then
    echo "Docker Compose plugin is required (docker compose ...)." >&2
    exit 1
  fi
}

ensure_env() {
  if [[ ! -f "$ENV_FILE" ]]; then
    log "No .env found. Creating from .env.example"
    cp "$ENV_EXAMPLE" "$ENV_FILE"
    log "Edit $ENV_FILE before first production run."
  fi
}

install_stack() {
  check_prereqs
  ensure_env
  log "Building and starting gateway container"
  docker compose -f "$COMPOSE_FILE" up -d --build
  log "Gateway is starting. Use: $0 logs"
}

update_stack() {
  check_prereqs
  log "Pulling latest code"
  git -C "$APP_DIR" pull --ff-only
  ensure_env
  log "Rebuilding and restarting gateway"
  docker compose -f "$COMPOSE_FILE" up -d --build
  log "Update complete"
}

status_stack() {
  check_prereqs
  docker compose -f "$COMPOSE_FILE" ps
}

logs_stack() {
  check_prereqs
  docker compose -f "$COMPOSE_FILE" logs -f gateway
}

restart_stack() {
  check_prereqs
  docker compose -f "$COMPOSE_FILE" restart gateway
}

down_stack() {
  check_prereqs
  docker compose -f "$COMPOSE_FILE" down
}

usage() {
  cat <<EOF
Usage: $0 <command>

Commands:
  install   Build and start the stack
  update    Git pull + rebuild + restart
  status    Show container status
  logs      Follow gateway logs
  restart   Restart gateway container
  down      Stop and remove containers
EOF
}

main() {
  local cmd="${1:-}"
  case "$cmd" in
    install) install_stack ;;
    update) update_stack ;;
    status) status_stack ;;
    logs) logs_stack ;;
    restart) restart_stack ;;
    down) down_stack ;;
    *) usage; exit 1 ;;
  esac
}

main "$@"
