#!/bin/bash

cleanup() {
    echo "Terminating processes..."
    kill 0
}

trap cleanup EXIT

# Start the otto server
go run main.go server --dev-mode 2>&1 | while IFS= read -r line; do
    printf "\033[38;5;183m[server]\033[0m %s\n" "$line"
done &
otto_pid=$!

# Start the admin UI
cd ui/admin && VITE_API_IN_BROWSER=true npm run dev 2>&1 | while IFS= read -r line; do
    printf "\033[38;5;153m[admin-ui]\033[0m %s\n" "$line"
done &
npm_pid=$!

# Wait for both processes to finish
wait "$otto_pid" "$npm_pid"
