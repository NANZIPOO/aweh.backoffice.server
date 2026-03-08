#!/bin/bash
# Server-side git hook: post-receive
# Location: ~/aweh.backoffice.server.git/hooks/post-receive
# This runs when you push code directly to the server

echo "======================================"
echo "  Direct Deploy Hook Triggered"
echo "======================================"
echo ""

# Set working directory
WORK_DIR="/home/pacha/aweh.backoffice.server"

echo "📂 Working directory: $WORK_DIR"
echo ""

# Navigate to working directory
cd "$WORK_DIR" || exit 1

# Unset GIT_DIR to avoid bare repository issues
unset GIT_DIR

echo "📥 Pulling latest code from bare repository..."
git --work-tree="$WORK_DIR" --git-dir="$WORK_DIR/.git" pull origin main
echo "✓ Code updated"
echo ""

echo "🐳 Rebuilding and restarting gateway container..."
docker compose down gateway
docker compose build --no-cache gateway
docker compose up -d gateway
echo "✓ Container restarted"
echo ""

echo "⏳ Waiting for gateway to start..."
sleep 8
echo ""

echo "📊 Container status:"
docker compose ps gateway
echo ""

echo "📋 Recent logs:"
docker compose logs --tail=15 gateway
echo ""

echo "======================================"
echo "  ✅ Deployment Complete!"
echo "======================================"
