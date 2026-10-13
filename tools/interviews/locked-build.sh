#!/usr/bin/env bash
# Run `astro build` while holding a lock, so several agents sharing this working
# tree never build at the same time (concurrent builds clobber dist/ and the
# content cache). Usage: tools/interviews/locked-build.sh [extra astro build args]
set -euo pipefail
cd "$(dirname "$0")/../.."

LOCK="${TMPDIR:-/tmp}/system-design-labs-astro-build.lock"
waited=0
until mkdir "$LOCK" 2>/dev/null; do
  # Break a stale lock left by a crashed build (older than 10 minutes).
  if [ -n "$(find "$LOCK" -maxdepth 0 -mmin +10 2>/dev/null)" ]; then
    rmdir "$LOCK" 2>/dev/null || true
    continue
  fi
  if [ $waited -eq 0 ]; then echo "waiting for another build to finish..."; fi
  sleep 2; waited=$((waited + 2))
done
trap 'rmdir "$LOCK" 2>/dev/null || true' EXIT

LOG="$(mktemp "${TMPDIR:-/tmp}/astro-build.XXXXXX")"
trap 'rmdir "$LOCK" 2>/dev/null || true; rm -f "$LOG"' EXIT
set +e
npx astro build "$@" > "$LOG" 2>&1
status=$?
set -e
first_error="$(grep -n '\[ERROR\]' "$LOG" | head -1 | cut -d: -f1 || true)"
if [ "$status" -ne 0 ] && [ -n "$first_error" ]; then
  # Show the whole error (integrity errors list every problem), not just the tail.
  sed -n "${first_error},$((first_error + 150))p" "$LOG"
else
  tail -40 "$LOG"
fi
# Interview warnings print early and would scroll out of the tail; repeat them once each.
if grep -q '\[interviews\] warning' "$LOG"; then
  echo "--- [interviews] warnings ---"
  grep '\[interviews\] warning' "$LOG" | sed 's/^.*\[interviews\] warning: //' | awk '!seen[$0]++'
fi
if [ "$status" -eq 0 ] && [ -f tools/check-links.mjs ]; then
  node tools/check-links.mjs
fi
exit "$status"
