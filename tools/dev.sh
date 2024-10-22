#!/bin/bash

set -e  # Exit on any command failure

print_section_header() {
    local color_code=$1
    local message=$2
    local terminal_width=$(tput cols || echo 80)  # Default to 80 if tput fails
    local padding_width=$(( (terminal_width - ${#message} - 2) / 2 ))
    local padding=$(printf '%*s' "$padding_width" '' | tr ' ' '-')

    echo -e "\033[38;5;${color_code}m${padding} ${message} ${padding}\033[0m"
}

cleanup() {
    kill 0
}

trap cleanup EXIT  # Handles script exit and Ctrl-C

print_section_header 120 "Starting otto8 server and admin UI..."

# Start the otto server
(
    go run main.go server --dev-mode 2>&1 | while IFS= read -r line; do
        printf "\033[38;5;183m[server]\033[0m %s\n" "$line"
    done
) &
otto_pid=$!

# Start the admin UI
(
    cd ui/admin
    VITE_API_IN_BROWSER=true npm run dev 2>&1 | while IFS= read -r line; do
        printf "\033[38;5;153m[admin-ui]\033[0m %s\n" "$line"
    done
) &
npm_pid=$!

# Health check and open the browser
(
    source .envrc.dev
    healthcheck_passed=false
    for _ in {1..60}; do  # ~1 minute timeout
        if kubectl get --raw /healthz &>/dev/null; then
            healthcheck_passed=true
            break
        fi
        sleep 1
    done

    if [[ "$healthcheck_passed" != true ]]; then
        print_section_header 196 "Timeout waiting for otto8 server to be ready"
        cleanup
    fi

    if command -v open >/dev/null; then
        open http://localhost:8080/admin/
    elif command -v xdg-open >/dev/null; then
        xdg-open http://localhost:8080/admin/
    else
        echo "Open http://localhost:8080/admin/ in your browser."
    fi
    print_section_header 120 "otto8 server and admin UI ready"
) &
healthcheck_pid=$!

# Wait for either otto or npm process to exit, then trigger cleanup
wait "$healthcheck_pid" "$otto_pid" "$npm_pid"
