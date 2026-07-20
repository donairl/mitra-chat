#!/usr/bin/env bash
# Run MitraChat backend (Go) + frontend (Vue/Vite) together.
# Ctrl-C stops both.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

cleanup() {
  echo ""
  echo "Stopping..."
  kill 0 2>/dev/null || true
}
trap cleanup EXIT INT TERM

# Backend
(
  cd "$ROOT/server"
  [ -f .env ] || { [ -f .env.example ] && cp .env.example .env && echo "[server] created .env from .env.example"; }
  echo "[server] go run . -> :3000"
  go run .
) &

# Frontend
(
  cd "$ROOT/client"
  [ -d node_modules ] || { echo "[client] installing deps..."; npm install; }
  echo "[client] npm run dev -> :5173"
  npm run dev
) &

wait
