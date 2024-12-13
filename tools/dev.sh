#!/bin/bash

set -e # Exit on any command failure

# Parse arguments for opening the user and admin UIs
open_uis=false

for arg in "$@"; do
  case $arg in
    --open-uis)
      open_uis=true
      ;;
  esac
done

print_with_color() {
  local color_code=$1
  local color_message=$2
  local uncolored_message=$3

  echo -e "\033[38;5;${color_code}m${color_message}\033[0m${uncolored_message}"
}

print_section_header() {
  local color_code=$1
  local message=$2
  local terminal_width=$(tput cols || echo 80) # Default to 80 if tput fails
  local padding_width=$(((terminal_width - ${#message} - 2) / 2))
  local padding=$(printf '%*s' "$padding_width" '' | tr ' ' '-')

  print_with_color "$color_code" "${padding} ${message} ${padding}"
}

open_browser_tabs() {
  if $open_uis; then
    if command -v open >/dev/null; then
      echo "$@" | xargs -n 1 open
    elif command -v xdg-open >/dev/null; then
      echo "$@" | xargs -n 1 xdg-open
    fi
  fi

  print_with_color 120 "UIs are accessible at: $(printf '%s ' "$@")"
}

cleanup() {
  kill 0
}

trap cleanup EXIT # Handles script exit and Ctrl-C

# Start the otto server
(
  print_section_header 183 "Starting server..."

  go run main.go server --dev-mode 2>&1 | while IFS= read -r line; do
    print_with_color 183 "[server]" " $line"
  done
) &
server_pid=$!

(
  source .envrc.dev

  for _ in {1..60}; do # ~1 minute timeout
    if kubectl get --raw /healthz &>/dev/null; then
      print_section_header 183 "Server ready!"
      exit 0
    fi
    sleep 1
  done

  print_section_header 196 "Timeout waiting for server to start"
  cleanup
) &
server_ready_pid=$!

(
  print_section_header 153 "Starting admin UI..."
  cd ui/admin
  pnpm i --ignore-scripts 2>&1 | while IFS= read -r line; do
    print_with_color 153 "[admin-ui](install)" " $line"
  done

  VITE_API_IN_BROWSER=true npm run dev 2>&1 | while IFS= read -r line; do
    print_with_color 153 "[admin-ui]" " $line"
  done
) &
admin_ui_pid=$!

(
  for _ in {1..60}; do # ~1 minute timeout
    if curl -s --head http://localhost:8080/admin/ | head -n 1 | grep "200 OK" > /dev/null; then
      print_section_header 153 "Admin UI ready!"
      exit
    fi
    sleep 1
  done

  print_section_header 196 "Timeout waiting for admin UI to start"
  cleanup
) &
admin_ui_ready_pid=$!

(
  print_section_header 217 "Starting user UI..."

  cd ui/user
  pnpm i 2>&1 | while IFS= read -r line; do
    print_with_color 217 "[user-ui](install)" " $line"
  done

  pnpm run dev --port 5174 2>&1 | while IFS= read -r line; do
    print_with_color 217 "[user-ui]" " $line"
  done
) &
user_ui_pid=$!

(
  for _ in {1..60}; do # ~1 minute timeout
    if curl -s --head http://localhost:8080/favicon.ico | head -n 1 | grep "200 OK" > /dev/null; then
      print_section_header 217 "User UI ready!"
      exit
    fi
    sleep 1
  done

  print_section_header 196 "Timeout waiting for user UI to start"
  cleanup
) &
user_ui_ready_pid=$!

wait "${server_ready_pid}" "${admin_ui_ready_pid}" "${user_ui_ready_pid}"
print_section_header 120 "All components ready!"

open_browser_tabs http://localhost:8080/

wait "${server_pid}" "${admin_ui_pid}" "${user_ui_pid}"
