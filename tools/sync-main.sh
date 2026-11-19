#!/usr/bin/env bash
# Copy only source main to destination main. Authentication belongs to the caller.
set -euo pipefail

if [[ $# != 2 ]]; then
    echo "Usage: $0 <source-repository> <destination-repository>" >&2
    exit 2
fi
source_repo=$1
destination_repo=$2
export GIT_TERMINAL_PROMPT=0

read_main() {
    local record sha
    record=$(git ls-remote --exit-code "$1" refs/heads/main) || return
    sha=${record%%$'\t'*}
    if [[ ! "$sha" =~ ^[0-9a-f]{40}$ || "$record" != "$sha"$'\trefs/heads/main' ]]; then
        echo "Expected exactly one main branch with a SHA-1 commit ID." >&2
        return 1
    fi
    printf '%s\n' "$sha"
}

# Observe the destination before fetching. A concurrent update must reject the
# push, even if it happens during the fetch. Never retry with a fresh lease here.
expected=$(read_main "$destination_repo")
scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT
git init --bare --quiet --template= "$scratch"
git -C "$scratch" fetch --quiet --no-tags "$source_repo" refs/heads/main
source_sha=$(git -C "$scratch" rev-parse --verify 'FETCH_HEAD^{commit}')

if [[ "$source_sha" != "$expected" ]]; then
    # An explicit refspec and disabled followTags preserve every other ref.
    # Sending the original commit preserves its exact tree and parent history.
    git -C "$scratch" -c push.followTags=false push --quiet \
        --force-with-lease="refs/heads/main:$expected" \
        "$destination_repo" "$source_sha:refs/heads/main"
fi

actual=$(read_main "$destination_repo")
if [[ "$actual" != "$source_sha" ]]; then
    echo "Destination main changed during verification; inspect it before rerunning." >&2
    exit 1
fi
printf 'Verified destination main matches upstream commit %s.\n' "$source_sha"
