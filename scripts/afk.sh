#!/usr/bin/env bash
# AFK driver for the Herdr desktop app. For each open `ready-for-agent` issue with no
# open blockers and no assignee: create a git worktree, start an interactive Claude
# session in herdr-core (hc), and show it in a new tab of the Herdr app window.
# Independent issues run in parallel; blocked ones wait for a re-run.
#
# Usage: scripts/afk.sh [--dry-run | --run] [--max N]
#   default   dry run: plans only, changes nothing
#   --run     live: assigns issues, creates worktrees, starts Claude, opens Herdr spaces
#   --max N   start at most N issues this run (default 3)
# Needs: the Herdr app running (herdr-app status), gh logged in as $AFK_GH_LOGIN (required, exit 2 otherwise).
# Logs: stdout plus $AFK_LOG_DIR/run-<timestamp>.log (default ~/.local/state/dynamis-afk).
set -euo pipefail

DRY_RUN=1
MAX=3
while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run) DRY_RUN=1 ;;
    --run) DRY_RUN=0 ;;
    --max) MAX=${2:?--max needs a number}; shift ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
  shift
done
LOG_DIR=${AFK_LOG_DIR:-$HOME/.local/state/dynamis-afk}
WT_ROOT=${AFK_WORKTREE_ROOT:-$HOME/.herdr/worktrees/dynamis-code-apps-template}
mkdir -p "$LOG_DIR" "$WT_ROOT"
RUN_LOG="$LOG_DIR/run-$(date +%Y%m%d-%H%M%S).log"

log() { # log LEVEL message...
  local line
  line="$(date '+%Y-%m-%d %H:%M:%S') $1 ${*:2}"
  echo "$line" | tee -a "$RUN_LOG"
}
trap 'log ERROR "script failed at line $LINENO (exit $?). See $RUN_LOG"' ERR

MODE=live
[ "$DRY_RUN" -eq 1 ] && MODE=dry-run
log INFO "run start mode=$MODE max=$MAX log=$RUN_LOG"

# Account preflight, in dry-run too: every write below must run as the expected login.
if [ -z "${AFK_GH_LOGIN:-}" ]; then
  log ERROR "AFK_GH_LOGIN is not set. Set it to the GitHub login this run must act as."
  exit 2
fi
GH_LOGIN=$(gh api user --jq .login)
if [ "$GH_LOGIN" != "$AFK_GH_LOGIN" ]; then
  log ERROR "gh acts as $GH_LOGIN but AFK_GH_LOGIN expects $AFK_GH_LOGIN. Run 'gh auth switch' and re-run."
  exit 2
fi
log INFO "gh account $GH_LOGIN matches AFK_GH_LOGIN"

if [ "$DRY_RUN" -eq 0 ]; then
  if ! herdr-app status >> "$RUN_LOG" 2>&1; then
    log ERROR "Herdr app window not reachable. Open Herdr, then re-run."
    exit 1
  fi
  log INFO "herdr-app window reachable"
fi

REPO=$(gh repo view --json nameWithOwner -q .nameWithOwner)
log INFO "repo $REPO"

started=0 skipped=0 candidates=0

# Fetch before the loop so a gh failure exits non-zero (errexit), not as "0 candidates".
issues=$(gh issue list --repo "$REPO" --state open --label ready-for-agent \
  --json number,title,assignees \
  --jq '.[] | select(.assignees | length == 0) | "\(.number)\t\(.title)"')

while IFS=$'\t' read -r n title; do
  [ -n "$n" ] || continue
  candidates=$((candidates + 1))
  # Resume safety first (local, no API call): a worktree means this issue already started.
  wt="$WT_ROOT/issue-$n"
  if [ -e "$wt" ]; then
    log SKIP "#$n worktree already exists at $wt, not starting twice: $title"
    skipped=$((skipped + 1))
    continue
  fi
  blockers=$(gh api "repos/$REPO/issues/$n" --jq .issue_dependencies_summary.blocked_by)
  if [ "$blockers" != 0 ]; then
    log SKIP "#$n blocked by $blockers open issue(s): $title"
    skipped=$((skipped + 1))
    continue
  fi
  if [ "$started" -ge "$MAX" ]; then
    log SKIP "max $MAX reached, #$n waits for the next run: $title"
    skipped=$((skipped + 1))
    continue
  fi
  if [ "$DRY_RUN" -eq 1 ]; then
    log PLAN "#$n would start: $title"
    started=$((started + 1))
    continue
  fi

  log INFO "#$n start: $title"
  gh issue edit "$n" --repo "$REPO" --add-assignee @me >> "$RUN_LOG" 2>&1 # prints the URL; keep it in the log only
  log INFO "#$n assigned to @me"

  git worktree add -b "issue-$n" "$wt" main >> "$RUN_LOG" 2>&1
  log INFO "#$n worktree $wt on branch issue-$n"

  # Interactive Claude in herdr-core. The task is its first message; hc waits for it to boot.
  hc run --bg --no-parent --name "issue-$n" --cwd "$wt" claude "/implement #$n

After the commit: run the verification steps you can run here and report results.
Open a PR into main with 'Closes #$n' in the body, using the repo PR template if one exists.
Do not edit CHANGELOG.md on this branch. No AI attribution lines in commits or PRs.
If a decision is needed from a human, comment on the issue and stop." >> "$RUN_LOG" 2>&1
  log INFO "#$n claude running in herdr-core as terminal issue-$n"

  # Show it in the Herdr app as its own space (not a tab or pane), attached to that terminal.
  herdr-app do space.new --name "#$n" --folder "$wt" \
    --argv "[\"hc\",\"attach\",\"issue-$n\"]" >> "$RUN_LOG" 2>&1
  log INFO "#$n shown in Herdr app as space \"#$n\""

  started=$((started + 1))
done <<< "$issues"

log INFO "run end candidates=$candidates started=$started skipped=$skipped"
