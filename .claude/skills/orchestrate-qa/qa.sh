#!/usr/bin/env bash
# qa.sh — the lead discobox's loop over the QA worker discoboxes it created, one issue each.
#
# State in $ORCH_DIR (default ~/.local/state/orchestrate; a scratchpad can be wiped):
#   queue.txt     issue numbers still to hand out, in order
#   workers.tsv   "<issue>\t<discobox-id>\t<state>"
#                 state: running | paused | passed | failed | could-not | done | broken | retired-<verdict>
#   verdicts.log  "<time>\t<issue>\t<discobox-id>\t<verdict line>"
#   tried.txt     credential request IDs already answered
#   approvals.log, launches.log, heartbeat.log, idle-seen.txt, idle-reported.txt
#
# Use IDs are looked up by their descriptions on every call, so a renewed grant just works.
# discobox-access always gets </dev/null: the judge is shown what a command reads on stdin.
# Run with bash, never source it into zsh: zsh does not word-split unquoted expansions.
set -uo pipefail

ORCH_DIR=${ORCH_DIR:-$HOME/.local/state/orchestrate}
REPO=${ORCH_REPO:-obot-platform/obot}
PROJECT_ORG=${ORCH_PROJECT_ORG:-obot-platform}
PROJECT_NUM=${ORCH_PROJECT_NUM:-2}
COLUMN=${ORCH_COLUMN:-Test Backlog}
MAX=${ORCH_MAX:-5}
SRC=${ORCH_SRC:-$(git -C "$(dirname "$0")" rev-parse --show-toplevel 2>/dev/null || pwd)}
LEASE=/run/discobox/keepalive/qa-orchestrator
MARKER='🤖 **AI-driven validation**'
mkdir -p "$ORCH_DIR"
W=$ORCH_DIR/workers.tsv Q=$ORCH_DIR/queue.txt
touch "$W" "$Q" "$ORCH_DIR/tried.txt" "$ORCH_DIR/idle-seen.txt" "$ORCH_DIR/idle-reported.txt"

# use <credential-name> <regex>: the live use ID whose description matches. The listing
# fails now and then for a moment, so an empty answer is asked again before it counts.
use() {
	local id i
	for i in 1 2 3; do
		id=$(discobox-access list --json </dev/null 2>/dev/null |
			jq -r --arg c "$1" --arg re "$2" '[.credentials[] | select(.name==$c) | .uses[] | select(.description|test($re))] | last | .useId // empty')
		[ -n "$id" ] && { echo "$id"; return; }
		[ "$i" -lt 3 ] && sleep 5
	done
}
need() { [ -n "$1" ] && return 0; echo "MISSING USE: no live use for $2 (SKILL.md §1)"; return 1; }
dbx() { local u; u=$(use ai.discobox.sandbox "$1"); need "$u" "$1" || return 1; shift; discobox-access run --use "$u" -- discobox "$@" </dev/null; }
gh_() { local u; u=$(use github "$1"); need "$u" "$1" || return 1; shift; discobox-access run --use "$u" -- gh "$@" </dev/null; }

active() { awk -F'\t' '$3=="running"' "$W"; }
id_of() { awk -F'\t' -v n="$1" '$1==n{print $2}' "$W" | tail -1; }
state_of() { awk -F'\t' -v n="$1" '$1==n{print $3}' "$W" | tail -1; }
set_state() { awk -F'\t' -v n="$1" -v s="$2" 'BEGIN{OFS="\t"} $1==n{$3=s} {print}' "$W" >"$W.tmp" && mv "$W.tmp" "$W"; }

# room: the pool's CPU, memory and per-box disk are shared by every box on it, not just ours.
# Launch only with headroom: load under the core count, memory free, and disk free — a lone
# worker may start on less disk, or a queue with nothing running would wait forever.
memfree_g() { awk '/MemAvailable/{print int($2/1048576)}' /proc/meminfo; }
diskfree_g() { df -BG --output=avail / | tail -1 | tr -dc 0-9; }
room() {
	local m l c d; m=$(memfree_g); l=$(cut -d' ' -f1 /proc/loadavg | cut -d. -f1); c=$(nproc); d=$(diskfree_g)
	[ "$m" -ge "${ORCH_MIN_MEM_G:-10}" ] && [ "$l" -lt "$c" ] &&
		{ [ "$d" -ge "${ORCH_MIN_LAUNCH_DISK_G:-15}" ] || { [ "$(active | wc -l)" -eq 0 ] && [ "$d" -ge "${ORCH_MIN_SOLO_DISK_G:-8}" ]; }; } && return 0
	echo "NO ROOM: ${m}G mem, load $l on $c cores, ${d}G disk"; return 1
}

cmd_backlog() { # backlog: write queue.txt from the project column, skipping issues already handled or already AI-validated
	local cursor="" out q all="" n
	q='query($org: String!, $num: Int!, $cursor: String) { organization(login: $org) { projectV2(number: $num) { items(first: 100, after: $cursor) {
	  pageInfo { hasNextPage endCursor }
	  nodes { status: fieldValueByName(name: "Status") { ... on ProjectV2ItemFieldSingleSelectValue { name } }
	          content { __typename ... on Issue { number title state repository { nameWithOwner } } } } } } } }'
	while :; do
		local args=(-f query="$q" -f org="$PROJECT_ORG" -F num="$PROJECT_NUM")
		[ -n "$cursor" ] && args+=(-f cursor="$cursor")
		out=$(gh_ '^gh api graphql' api graphql "${args[@]}" 2>&1) || { echo "$out" | tail -3; return 1; }
		all+=$(echo "$out" | jq -r --arg col "$COLUMN" --arg repo "$REPO" '.data.organization.projectV2.items.nodes[]
			| select(.status.name==$col and .content.__typename=="Issue" and .content.state=="OPEN" and .content.repository.nameWithOwner==$repo)
			| "\(.content.number)\t\(.content.title)"')$'\n'
		[ "$(echo "$out" | jq -r .data.organization.projectV2.items.pageInfo.hasNextPage)" = true ] || break
		cursor=$(echo "$out" | jq -r .data.organization.projectV2.items.pageInfo.endCursor)
	done
	: >"$Q.new"
	while IFS=$'\t' read -r n title; do
		[ -n "$n" ] || continue
		grep -q "^$n	" "$W" && continue
		if [ "${ORCH_SKIP_VALIDATED:-1}" = 1 ] && [ "$(comment_url "$n")" != "" ]; then echo "skip #$n: already has an AI validation"; continue; fi
		echo "$n" >>"$Q.new"; echo "#$n $title"
	done < <(printf '%s' "$all" | sort -n -u)
	mv "$Q.new" "$Q"
	echo "queue: $(wc -l <"$Q") issues — reorder queue.txt so locally testable ones go first (SKILL.md §2)"
}

# comment_url <issue>: the newest AI validation comment on the issue, or nothing
comment_url() {
	gh_ '^gh api GET repos/[^ ]+/issues/<number>' api "repos/$REPO/issues/$1/comments?per_page=100" \
		--jq "[.[] | select(.body|startswith(\"$MARKER\"))] | last | .html_url // empty" 2>/dev/null | tail -1
}

cmd_launch() { # launch [one]: start workers from the queue until MAX are running (or just one)
	local n out id
	while [ "$(active | wc -l)" -lt "$MAX" ]; do
		n=$(head -1 "$Q"); [ -n "$n" ] || { echo "queue empty"; return 0; }
		room || return 0
		out=$(cd "$SRC" && dbx '^discobox new' new -d --include-dirty=false -p "Validate issue #$n in $REPO." 2>&1)
		id=$(echo "$out" | grep -o 'sbx_[a-z0-9]*' | tail -1)
		if [ -z "$id" ]; then echo "LAUNCH FAILED #$n: $(echo "$out" | tail -3)"; return 1; fi
		sed -i 1d "$Q"
		printf '%s\t%s\trunning\n' "$n" "$id" >>"$W"
		dbx '^discobox tag <' tag "$id" qa "issue=$n" >/dev/null 2>&1
		echo "launched #$n $id"
		[ "${1:-}" = one ] && return 0
	done
}

cmd_status() {
	local j; j=$(dbx '^discobox admin box ls' admin box ls -o json 2>/dev/null) || { echo "$j"; return 1; }
	while IFS=$'\t' read -r n id st; do
		case "$st" in retired-*) [ "${1:-}" = all ] || continue ;; esac
		echo "$j" | jq -r --arg id "$id" --arg n "$n" --arg st "$st" '(.sandboxes // .)[] | select(.id==$id) |
			"#\($n) \($id) \($st) \(.runtime.runtimeState) \(if (.displayName|startswith("✳")) then "idle" else "busy" end)"'
	done <"$W"
	echo "queue: $(wc -l <"$Q") left · $(diskfree_g)G disk, $(memfree_g)G mem, load $(cut -d' ' -f1 /proc/loadavg) on $(nproc) cores"
}

cmd_screen() { dbx '^discobox admin terminal ls' admin terminal screen primary --discobox-id "$1" ${3:+--scrollback "$3"} 2>&1 | grep -v '^\s*$' | tail -"${2:-25}" | cut -c1-170; }
# say: answer a question or tell a worker to continue. Anything else (cleanup) is cmd_cleanup's use.
cmd_say() { local id=$1; shift; dbx '^discobox admin terminal input <terminal-id> --discobox-id <discobox-id> \[--literal\] <keys' admin terminal input primary --discobox-id "$id" "$@" 2>&1 | tail -1; }
cmd_power() { local v=$1; shift; dbx '^discobox start <' "$v" "$@" 2>&1 | tail -3; }

cmd_cleanup() { # cleanup <discobox-id>...: ask workers to free their own Docker disk once they have posted
	local id u; u=$(use ai.discobox.sandbox 'free its own Docker disk'); need "$u" "the cleanup message" || return 1
	for id in "$@"; do
		discobox-access run --use "$u" -- discobox admin terminal input primary --discobox-id "$id" --literal \
			"Disk is nearly full. Pull only the images you need. Once your validation comment is posted, before your final line, free this box's own Docker disk: docker rm -f \$(docker ps -aq); docker volume prune -a -f; docker image prune -a -f; docker builder prune -a -f." </dev/null >/dev/null 2>&1
		discobox-access run --use "$u" -- discobox admin terminal input primary --discobox-id "$id" Enter </dev/null 2>&1 | tail -1
	done
}

cmd_approve() { # approve each worker's com.github.api use request once; everything else is the lead's
	local reqs rid sid purpose wk n res
	reqs=$(dbx '^discobox secret request ls' secret request ls -o json 2>/dev/null |
		jq -r '.secretRequests[]? | select(.status=="pending" and .reason!="refresh") | "\(.id) \(.sandboxId) \(.purpose) \(.wellKnownId // "custom")"')
	while read -r rid sid purpose wk; do
		[ -z "$rid" ] && continue
		grep -qx "$rid" "$ORCH_DIR/tried.txt" && continue
		n=$(awk -F'\t' -v s="$sid" '$2==s{print $1}' "$W")
		if [ -n "$n" ] && [ "$purpose" = use ] && [ "$wk" = com.github.api ]; then
			res=$(dbx '^discobox secret request approve' secret request approve "$rid" 2>&1 | tail -3 | head -1 | cut -c1-400)
			echo "$(date -u +%FT%H:%M) approve $rid for #$n: $res" >>"$ORCH_DIR/approvals.log"
			case "$res" in
			GRANT*) echo "$rid" >>"$ORCH_DIR/tried.txt"; echo "approved #$n $rid" ;;
			*"could not be asked"* | *"502"* | *unavailable*) : ;; # the judge was down: try again next pass
			*) echo "$rid" >>"$ORCH_DIR/tried.txt"; echo "REFUSED #$n $rid: $res" ;;
			esac
		else
			echo "$rid" >>"$ORCH_DIR/tried.txt"
			echo "NEEDS LEAD: $rid from ${n:+#$n }$sid ($purpose $wk)"
		fi
	done <<<"$reqs"
}

cmd_retire() { # retire <issue>: tag the box with its verdict and to-delete, and stop it
	local n=$1 id st; id=$(id_of "$n"); st=$(state_of "$n")
	dbx '^discobox tag <' tag "$id" to-delete "verdict=$st" >/dev/null 2>&1
	cmd_power stop "$id" >/dev/null
	set_state "$n" "retired-$st"
	echo "#$n $id tagged to-delete verdict=$st and stopped"
}

cmd_verdicts() { # VERDICT for each running worker whose screen ends on its final line; retire it right away
	local n id st v
	while IFS=$'\t' read -r n id st; do
		v=$(cmd_screen "$id" 40 | grep -o "validated #$n: .*" | tail -1)
		[ -z "$v" ] && continue
		case "$v" in *"COULD NOT"*) st=could-not ;; *PASSED*) st=passed ;; *FAILED*) st=failed ;; *) st=done ;; esac
		set_state "$n" "$st"
		printf '%s\t%s\t%s\t%s\n' "$(date -u +%FT%H:%M)" "$n" "$id" "$v" >>"$ORCH_DIR/verdicts.log"
		cmd_retire "$n" >/dev/null
		echo "VERDICT #$n $id $v"
	done < <(active)
}

# firstrun: a restarted Claude Code worker asks once whether to make auto mode the default, and
# that dialog swallows whatever is typed next. Answer "No, keep bypass permissions".
firstrun() { cmd_screen "$1" 14 | grep -q 'Make auto mode your default' || return 1; cmd_say "$1" Down Enter >/dev/null; sleep 3; }

# wake <issue> <id>: start a worker that stopped itself mid-work (idle timeout), get it back to a
# prompt and tell it to continue. A box the pool has no runtime for cannot start: mark it broken
# and requeue its issue first, so it is launched again.
wake() {
	local n=$1 id=$2 out end
	out=$(cmd_power start "$id")
	if echo "$out" | grep -q 'no runtime'; then
		set_state "$n" broken; sed -i "1i $n" "$Q"; echo "BROKEN #$n $id: requeued"; return
	fi
	end=$((SECONDS + 300))
	until cmd_screen "$id" 6 2>/dev/null | grep -q '❯\|Enter to select' || [ $SECONDS -ge $end ]; do sleep 10; done
	firstrun "$id"
	cmd_say "$id" continue Enter >/dev/null
	echo "WOKE #$n $id"
}

cmd_idle() { # IDLE for a running worker idle two passes in a row (after clearing what it can itself)
	local j now busy n id st scr
	j=$(dbx '^discobox admin box ls' admin box ls -o json 2>/dev/null) || return 0
	now=$(echo "$j" | jq -r '(.sandboxes // .)[] | select(.runtime.runtimeState=="running" and (.displayName|startswith("✳"))) | .id')
	busy=$(echo "$j" | jq -r '(.sandboxes // .)[] | select(.runtime.runtimeState=="running" and (.displayName|startswith("✳")|not)) | .id')
	grep -Fxf <(echo "$now") "$ORCH_DIR/idle-reported.txt" >"$ORCH_DIR/idle-reported.tmp" || true
	mv "$ORCH_DIR/idle-reported.tmp" "$ORCH_DIR/idle-reported.txt"
	while IFS=$'\t' read -r n id st; do
		if echo "$j" | jq -e --arg id "$id" '(.sandboxes // .)[] | select(.id==$id and .runtime.runtimeState=="stopped")' >/dev/null; then
			wake "$n" "$id" >>"$ORCH_DIR/launches.log"; continue
		fi
		echo "$now" | grep -qx "$id" || continue
		grep -qx "$id" "$ORCH_DIR/idle-seen.txt" || continue
		grep -qx "$id" "$ORCH_DIR/idle-reported.txt" && continue
		scr=$(cmd_screen "$id" 8)
		# "Login expired" is not a real expiry; continue clears it. A spend limit is real: report it.
		if echo "$scr" | grep -qi 'spend limit\|usage limit'; then echo "$id" >>"$ORCH_DIR/idle-reported.txt"; echo "LIMIT #$n $id"; continue; fi
		if echo "$scr" | grep -qi 'login expired'; then cmd_say "$id" continue Enter >/dev/null; continue; fi
		firstrun "$id" && continue
		if ! echo "$scr" | grep -q 'Enter to select' && echo "$scr" | grep -qE 'esc to interrupt|still running|· [0-9]+ (shells?|monitors?)'; then continue; fi
		echo "$id" >>"$ORCH_DIR/idle-reported.txt"
		echo "IDLE #$n $id"
	done < <(active)
	echo "$now" >"$ORCH_DIR/idle-seen.txt"
}

cmd_pause() { # pause: stop every running worker (a spend limit, a full pool); their work survives
	local n id st
	while IFS=$'\t' read -r n id st; do cmd_power stop "$id" >/dev/null; set_state "$n" paused; echo "paused #$n $id"; done < <(active)
}

cmd_resume() { # resume: start paused workers one a minute and tell each to continue
	local n id st
	while IFS=$'\t' read -r n id st; do
		set_state "$n" running
		wake "$n" "$id"
		sleep 45
	done < <(awk -F'\t' '$3=="paused"' "$W")
}

cmd_results() { # results: every verdict with its comment URL, failures first
	local n id st url
	while IFS=$'\t' read -r n id st; do
		st=${st#retired-}
		case "$st" in passed | failed | could-not | done) ;; *) continue ;; esac
		url=$(comment_url "$n")
		printf '%s\t#%s\t%s\n' "$st" "$n" "${url:-(no comment found)}"
	done <"$W" | awk -F'\t' '{r=($1=="failed")?1:($1=="could-not")?2:3; print r"\t"$0}' | sort -t$'\t' -k1,1n -k3,3V | cut -f2-
}

cmd_watch() { # watch: approve, launch, wake and retire on its own; exit only on what needs the lead
	local next=$((SECONDS + 1800)) out
	touch -d '+2 hours' "$LEASE" 2>/dev/null
	while :; do
		out=$(cmd_approve); [ -n "$out" ] && echo "$out"
		echo "$out" | grep -qE '^(REFUSED|NEEDS LEAD|MISSING USE)' && exit 0
		out=$(cmd_verdicts); [ -n "$out" ] && { echo "$out"; exit 0; }
		cmd_launch one 2>&1 | grep '^launched\|^LAUNCH FAILED' >>"$ORCH_DIR/launches.log"
		out=$(cmd_idle)
		if [ -n "$out" ]; then
			echo "$out" | while read -r what n id; do echo "$what $n $id"; cmd_screen "$id" 15 | sed 's/^/    /'; done
			exit 0
		fi
		[ "$(diskfree_g)" -lt "${ORCH_MIN_DISK_G:-3}" ] && { echo "DISK LOW: $(diskfree_g)G free"; exit 0; }
		[ "$(memfree_g)" -lt "${ORCH_LOW_MEM_G:-5}" ] && { echo "MEM LOW: $(memfree_g)G available"; exit 0; }
		if [ "$(active | wc -l)" -eq 0 ] && [ ! -s "$Q" ]; then echo "ALL DONE"; rm -f "$LEASE"; exit 0; fi
		# Heartbeat: log and keep going. Exiting on a timer made a missed wake-up stall every worker.
		if [ $SECONDS -ge $next ]; then
			echo "$(date -u +%FT%H:%M) $(diskfree_g)G disk, $(memfree_g)G mem, load $(cut -d' ' -f1 /proc/loadavg), $(active | wc -l) running, $(wc -l <"$Q") queued" >>"$ORCH_DIR/heartbeat.log"
			touch -d '+2 hours' "$LEASE" 2>/dev/null
			next=$((SECONDS + 1800))
		fi
		sleep 60
	done
}

case "${1:-}" in
backlog | launch | status | screen | say | power | cleanup | approve | verdicts | retire | idle | pause | resume | results | watch) c=$1; shift; "cmd_$c" "$@" ;;
*) sed -n '2,15p' "$0"; echo "usage: $0 backlog|launch|status [all]|screen|say|power|cleanup|approve|verdicts|retire|idle|pause|resume|results|watch"; exit 2 ;;
esac
