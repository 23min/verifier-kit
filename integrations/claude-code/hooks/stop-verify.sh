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
#
# Besides what ./verify needs, it needs only Bash and the usual Unix tools (cat, grep, head,
# rm), and no Python (#62). It keeps its count in the git folder, or in the project folder
# outside git. The session is CLAUDE_CODE_SESSION_ID, which Claude Code sets for hooks;
# without it, every session shares one count, and the hook says so.
set -uo pipefail

# json_string TEXT : TEXT as a JSON string, quotes included. Escapes what JSON requires: the
# quote, the backslash and every control character (a Bash string holds no NUL).
json_string() {
  local s=$1 i hex c
  s=${s//\\/\\\\}
  s=${s//\"/\\\"}
  s=${s//$'\n'/\\n}
  s=${s//$'\r'/\\r}
  s=${s//$'\t'/\\t}
  for i in {1..31}; do
    printf -v hex %02x "$i"
    printf -v c "\\x$hex"
    s=${s//"$c"/\\u00$hex}
  done
  printf '"%s"' "$s"
}

block() {  # block REASON : keep Claude from stopping, with REASON as its feedback
  printf '{"decision": "block", "reason": %s}\n' "$(json_string "$1")"
  exit 0
}

cat > /dev/null   # Claude Code's input; the hook needs nothing from it

session="${CLAUDE_CODE_SESSION_ID:-}"
session="${session//[^A-Za-z0-9_-]/}"
shared=""
if [ -z "$session" ]; then
  session=shared
  shared=" Claude Code gave no usable session id (CLAUDE_CODE_SESSION_ID), so every session shares this count."
fi

cd "${CLAUDE_PROJECT_DIR:-.}" || exit 0
max="${VERIFY_MAX_ATTEMPTS:-5}"
counter="$(git rev-parse --git-dir 2>/dev/null || echo .)/verify-attempts-$session"

if out="$(./verify 2>&1)"; then
  rm -f "$counter"
  exit 0
fi

n=$(( $(cat "$counter" 2>/dev/null || echo 0) + 1 ))
echo "$n" > "$counter"

if [ "$n" -gt "$max" ]; then
  rm -f "$counter"
  printf '{"systemMessage": %s}\n' "$(json_string "./verify still fails and the attempt cap ($max) is reached; Claude stopped so a person can look. CI will refuse this change as it stands.$shared")"
  exit 0
fi

# Keep the feedback short: the failing steps and their error lines only. -a: GNU grep would
# leave out a line that isn't valid UTF-8.
details="$(printf '%s\n' "$out" | grep -a -E '^(FAIL|skip|--)|^      (error|warning)' | head -40)"
block "./verify failed (attempt $n of $max).$shared Change only src/; the spec, checks, conformance adapter and tools are fixed for you. If the spec itself looks wrong, stop and say so instead of working around it.
$details"
