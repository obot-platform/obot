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

warn() {
    if [[ ${GITHUB_ACTIONS:-} == true ]]; then
        printf '::warning::%s\n' "$*"
    else
        printf 'Warning: %s\n' "$*" >&2
    fi
}

# Print the commit ID of main in repository $1, named $2 in error messages.
read_main() {
    local records record sha status=0
    records=$(git ls-remote --exit-code "$1" refs/heads/main) || status=$?
    # Exit status 2 only means that no ref matched.
    if (( status != 0 && status != 2 )); then
        echo "Could not list main in the $2 repository $1." >&2
        return 1
    fi
    # ls-remote matches patterns by suffix, so it also lists branches such as
    # backup/refs/heads/main. Keep only the exact ref.
    record=$(awk -F '\t' '$2 == "refs/heads/main"' <<<"$records")
    if [[ -z "$record" ]]; then
        echo "The $2 repository $1 has no main branch." >&2
        return 1
    fi
    sha=${record%%$'\t'*}
    if [[ ! "$sha" =~ ^[0-9a-f]{40}$ || "$record" != "$sha"$'\trefs/heads/main' ]]; then
        echo "Expected one main branch with a SHA-1 commit ID in the $2 repository $1." >&2
        return 1
    fi
    printf '%s\n' "$sha"
}

# Observe the destination before fetching. A concurrent update must reject the
# push, even if it happens during the fetch. Never retry with a fresh lease here.
expected=$(read_main "$destination_repo" destination)
source_sha=$(read_main "$source_repo" source)

# Fetch only when main differs, so days without upstream changes download nothing.
if [[ "$source_sha" != "$expected" ]]; then
    scratch=$(mktemp -d)
    trap 'rm -rf "$scratch"' EXIT
    git init --bare --quiet --template= "$scratch"
    git -C "$scratch" fetch --quiet --no-tags "$source_repo" refs/heads/main
    # Upstream may have advanced since ls-remote; sync the commit actually fetched.
    source_sha=$(git -C "$scratch" rev-parse --verify 'FETCH_HEAD^{commit}')

    if [[ "$source_sha" != "$expected" ]]; then
        # The fetched history contains the destination commit only if it is upstream.
        if git -C "$scratch" merge-base --is-ancestor "$expected" "$source_sha" 2>/dev/null; then
            printf 'Fast-forwarding destination main from %s to %s.\n' "$expected" "$source_sha"
        else
            warn "Destination main $expected is not in upstream history. Replacing it with $source_sha discards any commits made only on destination main."
        fi
        # The explicit refspec updates only main. Sending the original commit
        # preserves its exact tree and parent history.
        git -C "$scratch" push --quiet \
            --force-with-lease="refs/heads/main:$expected" \
            "$destination_repo" "$source_sha:refs/heads/main"
    fi
fi

actual=$(read_main "$destination_repo" destination)
if [[ "$actual" != "$source_sha" ]]; then
    echo "Destination main changed during verification; inspect it before rerunning." >&2
    exit 1
fi
printf 'Verified destination main matches upstream commit %s.\n' "$source_sha"
