#!/usr/bin/env bash
# Helpers for the "Obot AI" GitHub project board (obot-platform org project 2).
#
#   board.sh columns                          item count per Status column
#   board.sh list <Status>                    items in a column: repo#num, type, title
#   board.sh dump <Status> <out-file>         title, metadata, body and comments for every item in a column
#   board.sh show <owner/repo> <num>...       assignees, milestone and board status
#   board.sh status <owner/repo> <num> <Status>   move an item to a column (adds it to the board if missing)
#   board.sh member <name>                    org members whose login or display name matches
#   board.sh milestones <title>               due date of a milestone in every repo that has it
set -euo pipefail

ORG=obot-platform
PROJECT=2
BOARD_TITLE="Obot AI"
REPOS=(obot field-issues security-issues ops-and-it-issues)

items_json() {
	gh project item-list "$PROJECT" --owner "$ORG" --limit 2000 --format json
}

cmd=${1:-}
shift || true

case "$cmd" in
columns)
	items_json | jq -r '[.items[].status] | group_by(.) | map("\(.[0] // "(none)")\t\(length)") | .[]'
	;;

list)
	status=${1:?usage: board.sh list <Status>}
	items_json | jq -r --arg s "$status" \
		'.items[] | select(.status==$s) | "\(.content.repository)#\(.content.number)\t\(.content.type)\t\(.content.title)"'
	;;

dump)
	status=${1:?usage: board.sh dump <Status> <out-file>}
	out=${2:?usage: board.sh dump <Status> <out-file>}
	: >"$out"
	items_json | jq -r --arg s "$status" \
		'.items[] | select(.status==$s) | "\(.content.repository)\t\(.content.number)\t\(.content.type)"' |
		while IFS=$'\t' read -r repo n type; do
			echo "=================== $repo#$n [$type]" >>"$out"
			if [ "$type" = "PullRequest" ]; then
				gh pr view "$n" -R "$repo" --json title,state,isDraft,author,createdAt,mergedAt,milestone,assignees,reviewDecision,additions,deletions,body \
					--jq '"TITLE: \(.title)\nSTATE: \(.state) draft=\(.isDraft) author=\(.author.login) created=\(.createdAt[0:10]) merged=\(.mergedAt // "") milestone=\(.milestone.title // "") assignees=\([.assignees[].login]|join(",")) review=\(.reviewDecision) size=+\(.additions)/-\(.deletions)\n\n\(.body)"' >>"$out"
			else
				gh issue view "$n" -R "$repo" --json title,state,author,createdAt,milestone,assignees,labels,body,comments \
					--jq '"TITLE: \(.title)\nSTATE: \(.state) author=\(.author.login) created=\(.createdAt[0:10]) milestone=\(.milestone.title // "") assignees=\([.assignees[].login]|join(",")) labels=\([.labels[].name]|join(","))\n\n\(.body)\n\n--- COMMENTS:\n\([.comments[] | "[\(.author.login) \(.createdAt[0:10])] \(.body)"] | join("\n---\n"))"' >>"$out"
				# Only obot PRs reference obot issues by number reliably; other repos' numbers collide.
				if [ "$repo" = "$ORG/obot" ]; then
					echo "--- PRS MENTIONING #$n:" >>"$out"
					gh pr list -R "$repo" --state all --search "$n in:body" --limit 5 \
						--json number,state,isDraft,mergedAt,title \
						--jq '.[] | "  #\(.number) \(.state) draft=\(.isDraft) merged=\(.mergedAt // "") \(.title)"' >>"$out"
				fi
			fi
			echo >>"$out"
		done
	echo "wrote $out"
	;;

show)
	repo=${1:?usage: board.sh show <owner/repo> <num>...}
	shift
	for n in "$@"; do
		gh issue view "$n" -R "$repo" --json number,state,assignees,milestone,projectItems |
			jq -r --arg b "$BOARD_TITLE" \
				'"\(.number) \(.state) assignees=\([.assignees[].login]|join(",")) milestone=\(.milestone.title // "") status=\([.projectItems[] | select(.title==$b) | .status.name]|join(","))"'
	done
	;;

status)
	repo=${1:?usage: board.sh status <owner/repo> <num> <Status>}
	n=${2:?usage: board.sh status <owner/repo> <num> <Status>}
	status=${3:?usage: board.sh status <owner/repo> <num> <Status>}
	project_id=$(gh project view "$PROJECT" --owner "$ORG" --format json --jq .id)
	field=$(gh project field-list "$PROJECT" --owner "$ORG" --format json | jq -c '.fields[] | select(.name=="Status")')
	field_id=$(jq -r .id <<<"$field")
	option_id=$(jq -r --arg s "$status" '.options[] | select(.name==$s) | .id' <<<"$field")
	if [ -z "$option_id" ]; then
		echo "unknown status '$status'; valid: $(jq -r '[.options[].name] | join(", ")' <<<"$field")" >&2
		exit 1
	fi
	item_id=$(items_json | jq -r --arg r "$repo" --argjson n "$n" \
		'.items[] | select(.content.repository==$r and .content.number==$n) | .id')
	if [ -z "$item_id" ]; then
		kind=issues
		gh pr view "$n" -R "$repo" >/dev/null 2>&1 && kind=pull
		item_id=$(gh project item-add "$PROJECT" --owner "$ORG" --url "https://github.com/$repo/$kind/$n" --format json --jq .id)
	fi
	gh project item-edit --project-id "$project_id" --id "$item_id" --field-id "$field_id" \
		--single-select-option-id "$option_id" --format json --jq .id >/dev/null
	echo "$repo#$n -> $status"
	;;

member)
	query=${1:?usage: board.sh member <name>}
	gh api --paginate "orgs/$ORG/members" --jq '.[].login' | while read -r login; do
		name=$(gh api "users/$login" --jq '.name // ""')
		printf '%s\t%s\n' "$login" "$name"
	done | grep -i -- "$query" || echo "no member matches '$query'" >&2
	;;

milestones)
	title=${1:?usage: board.sh milestones <title>}
	for r in "${REPOS[@]}"; do
		gh api "repos/$ORG/$r/milestones?state=all" |
			jq -r --arg t "$title" --arg r "$r" '.[] | select(.title==$t) | "\($r)\t#\(.number)\tdue=\(.due_on)\topen=\(.open_issues)\tstate=\(.state)"'
	done
	;;

*)
	sed -n '2,11p' "$0" | sed 's/^# \{0,1\}//'
	exit 1
	;;
esac
