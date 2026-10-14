#!/usr/bin/env bash
# Acceptance test for the toolkit: seeds one fault at a time into a scratch copy of this
# repo and checks that the right part catches it. Your working copy is never touched.
#
#   demo/faults.sh          run every scenario
#   demo/faults.sh 3        run scenario 3 only
#
# Exit 0 when every scenario behaves as expected, 1 otherwise. CI runs this on every push.
# Needs python3 with PyYAML (or a .venv) and git.
set -uo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SCRATCH="$(mktemp -d)"
trap 'rm -rf "$SCRATCH"' EXIT
W="$SCRATCH/repo"
ONLY="${1:-}"
FAILS=0
RAN=0
OUT=""
CODE=0

if [ -n "${PYTHON:-}" ]; then PY="$PYTHON"
elif [ -x "$ROOT/.venv/bin/python" ]; then PY="$ROOT/.venv/bin/python"
else PY=python3
fi
export PYTHON="$PY"   # ./verify inside the scratch copy uses the same interpreter

fresh() {                       # a clean copy of the repo to break
  rm -rf "$W"
  mkdir -p "$W"
  (cd "$ROOT" && tar --exclude=.git --exclude=.venv --exclude=reports --exclude=__pycache__ -cf - .) | (cd "$W" && tar -xf -)
}

scenario() {                    # scenario N TITLE EXPECTED
  [ -n "$ONLY" ] && [ "$ONLY" != "$1" ] && return 1
  RAN=$((RAN + 1))
  printf '\n=== %s. %s\n    expect: %s\n\n' "$1" "$2" "$3"
  fresh
  return 0
}

show() {                        # run a command in the scratch copy; keep its output and exit code
  OUT="$( (cd "$W" && "$@") 2>&1 )"
  CODE=$?
  printf '%s\n' "$OUT" | sed 's/^/    /'
  printf '    (exit %s)\n' "$CODE"
}

expect() {                      # expect EXIT [TEXT...]: the last show exited EXIT and printed each TEXT
  local want="$1" ok=1 text
  shift
  if [ "$CODE" != "$want" ]; then echo "    !! expected exit $want, got $CODE"; ok=0; fi
  for text in "$@"; do
    if ! printf '%s' "$OUT" | grep -qF -- "$text"; then echo "    !! expected output to contain: $text"; ok=0; fi
  done
  if [ "$ok" = 1 ]; then echo "    ok, as expected"; else FAILS=$((FAILS + 1)); fi
}

if scenario 0 "Baseline: nothing broken" "every step passes, all mutants killed"; then
  show ./verify --full
  expect 0 "all steps passed"
fi

if scenario 1 "The service as first written" \
   "conformance finds the two bugs, each with the shortest event sequence that shows it"; then
  cp "$W/examples/notes/demo/notes_app_as_written.py" "$W/examples/notes/src/notes_app.py"
  show ./verify
  expect 1 "diverges           draft/trash role=owner" "diverges           trashed/edit role=owner"
fi

if scenario 2 "Someone loosens the spine: editors may restore" \
   "three independent signals: an invalid behavior is now allowed, a check cites a changed cell, and the code no longer conforms"; then
  sed -i.bak \
    -e "s/{from: trashed, event: restore, when: \"role == 'owner'\", to: active}/{from: trashed, event: restore, when: \"role != 'viewer'\", to: active}/" \
    -e "s/{from: trashed, event: restore, when: \"role != 'owner'\", reject: true}/{from: trashed, event: restore, when: \"role == 'viewer'\", reject: true}/" \
    "$W/examples/notes/spec/notes/spine.yaml" && rm -f "$W/examples/notes/spec/notes/spine.yaml.bak"
  show ./verify
  expect 1 "behavior-allowed" "stale-check" "diverges           trashed/restore role=editor"
fi

if scenario 3 "A YAML slip: a rule written with 'on:' instead of 'event:'" \
   "spine check refuses the file and says why ('on' is read as true in YAML 1.1)"; then
  "$PY" - "$W/examples/notes/spec/notes/spine.yaml" <<'EOF'
import sys
p = sys.argv[1]
s = open(p).read()
open(p, "w").write(s.replace("event: publish", "on: publish", 1))
EOF
  show ./verify
  expect 1 "yaml-boolean-key" "skip  notes: other steps need a sound spine"
fi

if scenario 4 "The builder's branch also edits the spec" \
   "the path guard refuses the spec change; the code change is fine"; then
  (cd "$W" && git init -q -b main && git add -A && git -c user.name=demo -c user.email=demo@example.com commit -qm base \
     && git switch -qc build/purge-confirmation \
     && echo "# builder was here" >> examples/notes/src/notes_app.py \
     && echo "# and here" >> examples/notes/spec/notes/spine.yaml \
     && git -c user.name=demo -c user.email=demo@example.com commit -qam "builder change")
  show env VERIFY_BASE=main ./verify
  expect 1 "path-denied        examples/notes/spec/notes/spine.yaml"
fi

if scenario 5 "Two readings of the chapter disagree (interpretation loop, not a gate)" \
   "spine diff lists each disagreement with a witness: the question to put to the spec author"; then
  show "$PY" tools/spine.py diff examples/notes/spec/notes/readings/reading-a.yaml examples/notes/spec/notes/readings/reading-b.yaml
  expect 1 "case-disagrees     draft/trash: role=owner" "case-disagrees     trashed/restore: role=editor"
fi

if scenario 6 "Checking the checker: the content checks are deleted" \
   "the mutant that upper-cases text survives, because only the content checks could see it"; then
  rm "$W/examples/notes/checks/notes/test_content.py"
  show ./verify --full
  expect 1 "mutant-survived    edits are stored in upper case"
fi

if scenario 7 "The agent's Stop hook on a failing tree" \
   "first a block with the failing lines as feedback, then (after the attempt cap) a warning and a stop"; then
  cp "$W/examples/notes/demo/notes_app_as_written.py" "$W/examples/notes/src/notes_app.py"
  (cd "$W" && git init -q -b main)
  echo "    -- attempt 1"
  show env CLAUDE_PROJECT_DIR="$W" VERIFY_MAX_ATTEMPTS=1 \
    bash -c 'echo "{\"session_id\": \"demo\"}" | integrations/claude-code/hooks/stop-verify.sh'
  expect 0 '"decision": "block"' "draft/trash role=owner"
  echo "    -- attempt 2"
  show env CLAUDE_PROJECT_DIR="$W" VERIFY_MAX_ATTEMPTS=1 \
    bash -c 'echo "{\"session_id\": \"demo\"}" | integrations/claude-code/hooks/stop-verify.sh'
  expect 0 '"systemMessage"' "attempt cap (1) is reached"
fi

if scenario 8 "The adapter, the content checks and the mutation script are removed" \
   "each missing piece is an error; nothing is skipped silently"; then
  mv "$W/examples/notes/conformance/adapter_notes.py" "$W/examples/notes/conformance/adapter_notes.py.off"
  rm "$W/examples/notes/checks/notes/test_content.py"
  rm -r "$W/examples/notes/mutation"
  show ./verify --full
  expect 1 "no-adapter         notes: examples/notes/conformance/adapter_notes.py not found" \
    "no-checks          examples/notes/checks: no test_* functions found" \
    "no-mutation        examples/notes/mutation: no examples/notes/mutation/mutate_*.py"
fi

if scenario 9 "The service cannot be imported: it imports a module that does not exist" \
   "the adapter and the content checks each report it as a finding, not a traceback"; then
  echo "import no_such_module" >> "$W/examples/notes/src/notes_app.py"
  show ./verify examples/notes
  expect 1 "adapter-failed     examples/notes/conformance/adapter_notes.py: new() raised ModuleNotFoundError: No module named 'no_such_module'" \
    "check-unloadable   examples/notes/checks/notes/test_content.py: cannot be imported: ModuleNotFoundError"
fi

echo
if [ "$RAN" = 0 ]; then echo "-- no scenario named ${ONLY}"; exit 2; fi
if [ "$FAILS" = 0 ]; then
  echo "-- all scenarios behaved as expected"
else
  echo "-- ${FAILS} expectation(s) failed"
  exit 1
fi
