#!/usr/bin/env bash
# Acceptance test for the toolkit: seeds one fault at a time into a scratch copy of this
# repo and checks that the right part catches it. Your working copy is never touched.
#
#   demo/faults.sh          run every scenario
#   demo/faults.sh 3        run scenario 3 only
#
# Exit 0 when every scenario behaves as expected, 1 otherwise. CI runs this on every push.
# Needs python3 with PyYAML, Go, git, and elixir for scenarios 11-14.
set -uo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SCRATCH="$(mktemp -d)"
trap 'rm -rf "$SCRATCH"' EXIT
SPINE="$SCRATCH/spine"   # for the scenarios that run spine directly; ./verify builds its own
go -C "$ROOT/tools" build -trimpath -o "$SPINE" ./cmd/spine || { echo "faults: cannot build spine" >&2; exit 1; }
W="$SCRATCH/repo"
ONLY="${1:-}"
FAILS=0
RAN=0
OUT=""
CODE=0

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
  python3 - "$W/examples/notes/spec/notes/spine.yaml" <<'EOF'
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
  show "$SPINE" diff examples/notes/spec/notes/readings/reading-a.yaml examples/notes/spec/notes/readings/reading-b.yaml
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
  show env CLAUDE_PROJECT_DIR="$W" CLAUDE_CODE_SESSION_ID=demo VERIFY_MAX_ATTEMPTS=1 \
    bash -c 'echo "{\"session_id\": \"demo\"}" | integrations/claude-code/hooks/stop-verify.sh'
  expect 0 '"decision": "block"' "draft/trash role=owner"
  echo "    -- attempt 2"
  show env CLAUDE_PROJECT_DIR="$W" CLAUDE_CODE_SESSION_ID=demo VERIFY_MAX_ATTEMPTS=1 \
    bash -c 'echo "{\"session_id\": \"demo\"}" | integrations/claude-code/hooks/stop-verify.sh'
  expect 0 '"systemMessage"' "attempt cap (1) is reached"
fi

if scenario 8 "The adapter, the content checks and the mutation script are removed" \
   "each missing piece is an error; nothing is skipped silently"; then
  rm "$W/examples/notes/conformance/adapter_notes.cmd" "$W/examples/notes/checks/run.cmd"
  rm -r "$W/examples/notes/mutation"
  show ./verify --full
  expect 1 "no-adapter         notes: examples/notes/conformance/adapter_notes.cmd not found" \
    "no-checks          examples/notes/checks: no examples/notes/checks/run.cmd" \
    "no-mutation        examples/notes/mutation: no examples/notes/mutation/mutate_*.cmd"
fi

if scenario 9 "The service cannot be imported: it imports a module that does not exist" \
   "the adapter and the content checks each report it as a finding, not a traceback"; then
  echo "import no_such_module" >> "$W/examples/notes/src/notes_app.py"
  show ./verify examples/notes
  expect 1 "adapter-failed     env PYTHONPATH=src adapter_shim.py conformance/adapter_notes.py: new() raised ModuleNotFoundError: No module named 'no_such_module'" \
    "check-unloadable   checks/notes/test_content.py: cannot be imported: ModuleNotFoundError"
fi

# Scenario 10 ran notes as first written through the adapter protocol; since #69 notes' adapter
# is a command line, so scenario 1 does that. Its number stays unused, so that earlier citations
# of the scenarios after it stay right.

# Scenarios 11-13 seed faults in the elixir_job example, whose pieces are .cmd commands.
JOB="examples/elixir_job/src/job.ex"

if scenario 11 "Elixir: the job retries after its last attempt fails" \
   "conform, through adapter_job.cmd, finds the divergence"; then
  sed -i.bak 's/def transition(:running, :fail, :last), do: {:ok, :discarded}/def transition(:running, :fail, :last), do: {:ok, :retrying}/' "$W/$JOB"
  show ./verify examples/elixir_job
  expect 1 "diverges           running/fail attempts=last"
fi

if scenario 12 "Elixir: the job counts one attempt too many" \
   "the lifecycle still conforms; a content check through checks/run.cmd fails"; then
  sed -i.bak 's/if current == job.max_attempts/if current > job.max_attempts/' "$W/$JOB"
  show ./verify examples/elixir_job
  expect 1 "ok    job: code conforms to spine" "FAIL  content checks" "test J03: maximum 3"
fi

if scenario 13 "Elixir: checking the checker, the content checks are deleted" \
   "a counting mutant survives, since conform alone cannot see it"; then
  rm "$W/examples/elixir_job/checks/job/job_test.exs"
  show ./verify examples/elixir_job --full
  expect 1 "no checks ran from checks/job/" "mutant-survived    one attempt too many"
fi

if scenario 14 "Checking the checker: every example's code already fails its oracle" \
   "each mutation script stops with oracle-fails, rather than counting every mutant as caught"; then
  cp "$W/examples/notes/demo/notes_app_as_written.py" "$W/examples/notes/src/notes_app.py"
  sed -i.bak 's/self.remote.push(parent, files, force=False)/self.remote.push(parent, files, force=True)/' \
    "$W/examples/ref_sync/src/ref_sync_client.py"
  sed -i.bak 's/def transition(:running, :fail, :last), do: {:ok, :discarded}/def transition(:running, :fail, :last), do: {:ok, :retrying}/' "$W/$JOB"
  show ./verify --full
  expect 1 "oracle-fails       on src/notes_app.py as it is: conformance did not pass" \
    "oracle-fails       on src/ref_sync_client.py as it is: checks did not pass" \
    "oracle-fails       on the job as it is"
fi

# The Stop hook's reply, read as JSON, as Claude Code reads it.
READ_REPLY='import json, sys
for key, value in json.loads(sys.argv[1]).items():
    print(f"{key}: {value}")'

if scenario 15 "The agent's Stop hook on a machine without python3, nor a session id" \
   "./verify fails, and the hook still blocks with the failing lines, then warns at the attempt cap; both say every session shares the count"; then
  (cd "$W" && git init -q -b main)
  NOPY="$SCRATCH/no-python3"   # every program on PATH but python3, found as PATH would
  mkdir -p "$NOPY"
  IFS=: read -ra path_dirs <<< "$PATH"
  for d in "${path_dirs[@]}"; do
    for f in "$d"/*; do
      case "${f##*/}" in python3*) continue ;; esac
      [ -f "$f" ] && [ -x "$f" ] && [ ! -e "$NOPY/${f##*/}" ] && ln -s "$f" "$NOPY/${f##*/}"
    done
  done
  for attempt in 1 2; do
    echo "    -- attempt $attempt"
    show env -u CLAUDE_CODE_SESSION_ID PATH="$NOPY" CLAUDE_PROJECT_DIR="$W" VERIFY_MAX_ATTEMPTS=1 \
      bash -c 'echo "{}" | integrations/claude-code/hooks/stop-verify.sh'
    expect 0
    show python3 -c "$READ_REPLY" "$OUT"
    if [ "$attempt" = 1 ]; then
      expect 0 "decision: block" "FAIL  content checks" "every session shares this count"
    else
      expect 0 "systemMessage: ./verify still fails and the attempt cap (1) is reached" "every session shares this count"
    fi
  done
fi

if scenario 16 "The agent's Stop hook on failing lines with quotes, backslashes, control characters and a byte that isn't UTF-8" \
   "the reply is JSON, and its failing lines are exactly the ones ./verify printed"; then
  cat > "$W/verify" <<'EOF'
#!/usr/bin/env bash
printf 'ok    left out of the feedback\n'
printf 'FAIL  "quotes", a backslash \\ and \\n as text\n'
printf '      error   a tab\t, a bell\a, an escape \e[31m, a unit separator \x1f, a carriage return \r here\n'
printf '      warning caf\xc3\xa9, \xe2\x98\x83, and a byte that is not UTF-8: \xff\n'
printf -- '-- 1 step(s) failed\n'
exit 1
EOF
  show env CLAUDE_PROJECT_DIR="$W" CLAUDE_CODE_SESSION_ID=demo \
    bash -c 'echo "{}" | integrations/claude-code/hooks/stop-verify.sh'
  show python3 -c 'import json, sys
want = [b"FAIL  \"quotes\", a backslash \\ and \\n as text",
        b"      error   a tab\t, a bell\a, an escape \x1b[31m, a unit separator \x1f, a carriage return \r here",
        b"      warning caf\xc3\xa9, \xe2\x98\x83, and a byte that is not UTF-8: \xff",
        b"-- 1 step(s) failed"]
reply = json.loads(sys.argv[1])
got = reply["reason"].encode("utf-8", "surrogateescape").split(b"\n")[1:]
print("the failing lines are as printed" if got == want else f"they differ: {got!r}")' "$OUT"
  expect 0 "the failing lines are as printed"
fi

if scenario 17 "The agent's Stop hook when the project folder it is given doesn't exist" \
   "a block saying ./verify did not run, rather than letting the builder stop"; then
  show env CLAUDE_PROJECT_DIR="$W/gone" CLAUDE_CODE_SESSION_ID=demo \
    bash -c 'echo "{}" | integrations/claude-code/hooks/stop-verify.sh'
  show python3 -c "$READ_REPLY" "$OUT"
  expect 0 "decision: block" "cannot enter the project folder" "so ./verify did not run"
fi

echo
if [ "$RAN" = 0 ]; then echo "-- no scenario named ${ONLY}"; exit 2; fi
if [ "$FAILS" = 0 ]; then
  echo "-- all scenarios behaved as expected"
else
  echo "-- ${FAILS} expectation(s) failed"
  exit 1
fi
