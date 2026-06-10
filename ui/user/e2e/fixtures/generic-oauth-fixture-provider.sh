#!/bin/sh
set -eu

case "$0" in
	*/*) script_dir=${0%/*} ;;
	*) script_dir=. ;;
esac

for node_bin in "${NODE:-}" "$(command -v node 2>/dev/null || true)" /opt/homebrew/bin/node /usr/local/bin/node /usr/bin/node; do
	if [ -n "$node_bin" ] && [ -x "$node_bin" ]; then
		exec "$node_bin" "$script_dir/bin/generic-oauth-fixture-provider.mjs" "$@"
	fi
done

echo "node executable not found" >&2
exit 127
