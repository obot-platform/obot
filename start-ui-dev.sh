#!/bin/bash

# Script to start the frontend UIs as separate npm processes

set -e

echo "Starting Admin UI (React Router) on port 5173..."
cd ui/admin
pnpm install --ignore-scripts
VITE_API_IN_BROWSER=true pnpm run dev &
ADMIN_UI_PID=$!
echo "Admin UI started with PID: $ADMIN_UI_PID"

echo ""
echo "Starting User UI (SvelteKit) on port 5174..."
cd ../user
pnpm install
pnpm run dev --port 5174 &
USER_UI_PID=$!
echo "User UI started with PID: $USER_UI_PID"

echo ""
echo "========================================="
echo "Frontend UIs are running:"
echo "  Admin UI: http://localhost:5173"
echo "  User UI:  http://localhost:5174"
echo ""
echo "The Go server (with OBOT_DEV_MODE=true) will proxy:"
echo "  /legacy-admin/* → http://localhost:5173"
echo "  /*              → http://localhost:5174"
echo ""
echo "Press Ctrl+C to stop both servers"
echo "========================================="

# Wait for both processes
wait $ADMIN_UI_PID $USER_UI_PID

