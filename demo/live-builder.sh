#!/usr/bin/env bash
# A live builder run with Claude Code, in a scratch copy of this repo.
#
# The service starts as first written (two bugs). The builder is told about one bug only
# and cannot run shell commands, so it cannot run ./verify itself. The Stop hook from
# examples/claude-code runs ./verify when the builder tries to finish and sends it back
# with the failing case. Your working copy is never touched.
#
# Needs the claude CLI, logged in. Costs one short agent session (about $0.25 with Sonnet
# when we tried it). The scratch copy is kept so you can inspect it.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
W="$(mktemp -d)/repo"
MODEL="${MODEL:-sonnet}"

cp -r "$ROOT" "$W"
cd "$W"
rm -rf .git reports
cp demo/notes_app_as_written.py src/notes_app.py
mkdir -p .claude
cp examples/claude-code/settings.json .claude/settings.json
git init -q -b main
git add -A
git -c user.name=demo -c user.email=demo@example.com commit -qm "start: service as first written"
git switch -qc build/trash

echo "before: $(./verify | tail -1)"
VERIFY_MAX_ATTEMPTS=3 claude -p \
  "Users report that they can edit notes that are in the trash. Fix that in src/notes_app.py. Keep the change small." \
  --model "$MODEL" --permission-mode acceptEdits \
  --allowedTools "Read" "Grep" "Glob" --disallowedTools "Bash" \
  --output-format stream-json --verbose > run.jsonl

python3 - <<'EOF'
import json
for line in open("run.jsonl"):
    m = json.loads(line)
    if m.get("type") == "system" and m.get("subtype") == "hook_response" and m.get("hook_event") == "Stop":
        out = m.get("stdout", "").strip()
        if out:
            errors = [l.strip() for l in json.loads(out).get("reason", "").splitlines() if l.strip().startswith("error")]
            print("stop hook: blocked ->", "; ".join(errors))
        else:
            print("stop hook: passed, builder may stop")
    elif m.get("type") == "result":
        print("builder:", (m.get("result") or "").splitlines()[0])
EOF
echo "after:  $(./verify | tail -1)"
git --no-pager diff --stat
echo "scratch copy: $W"
