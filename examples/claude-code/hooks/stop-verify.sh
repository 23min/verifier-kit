#!/usr/bin/env bash
# Claude Code Stop hook (example): the builder may not finish while ./verify fails.
#
# When ./verify fails, the hook answers with {"decision": "block", "reason": ...}, which
# sends Claude back to work with the failing lines as its feedback. It uses JSON on stdout
# rather than exit 2, because Claude Code treats an exit-2 Stop hook whose stderr mentions
# a missing file as a non-blocking error, and verify output can contain such text.
#
# Claude Code's own cap (8 continuations in a row) resets whenever Claude calls a tool, so
# a builder that keeps editing could loop for a long time. This hook therefore counts its
# own attempts per session and lets Claude stop after VERIFY_MAX_ATTEMPTS (default 5),
# with a warning to the person. CI still refuses the change; the cap only ends the session.
set -uo pipefail

input="$(cat)"
session="$(printf '%s' "$input" | python3 -c 'import json, sys; print(json.load(sys.stdin).get("session_id", "unknown"))' 2>/dev/null || echo unknown)"

cd "${CLAUDE_PROJECT_DIR:-.}" || exit 0
max="${VERIFY_MAX_ATTEMPTS:-5}"
counter="$(git rev-parse --git-dir 2>/dev/null || echo .)/verify-attempts-${session//[^A-Za-z0-9_-]/}"

if out="$(./verify 2>&1)"; then
  rm -f "$counter"
  exit 0
fi

n=$(( $(cat "$counter" 2>/dev/null || echo 0) + 1 ))
echo "$n" > "$counter"

if [ "$n" -gt "$max" ]; then
  rm -f "$counter"
  python3 -c 'import json, sys; print(json.dumps({"systemMessage": sys.argv[1]}))' \
    "./verify still fails and the attempt cap ($max) is reached; Claude stopped so a person can look. CI will refuse this change as it stands."
  exit 0
fi

# Keep the feedback short: the failing steps and their error lines only.
details="$(printf '%s\n' "$out" | grep -E '^(FAIL|skip|--)|^      (error|warning)' | head -40)"
reason="./verify failed (attempt $n of $max). Change only src/; the spec, checks, conformance adapter and tools are fixed for you. If the spec itself looks wrong, stop and say so instead of working around it.
$details"
python3 -c 'import json, sys; print(json.dumps({"decision": "block", "reason": sys.argv[1]}))' "$reason"
exit 0
