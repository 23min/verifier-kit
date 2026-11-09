# Using verifier-kit in your own project

This guide takes a project from nothing to gated builds: an oracle you own (a spine, its checks
and an adapter) and a builder agent that may change only the code, checked by `./verify` from
an agent hook, a git hook and CI. It says, step by step, what an LLM can do for you and what
you have to do yourself, and why.

The split follows the design's principles (`docs/design.md`):

- **You own the oracle; the builder never touches it** (principle 3). Everything that judges the
  code, the chapter, spine, behaviors, adapter, checks and mutants, is yours, however much of it
  an LLM drafts.
- **Models may raise doubts, never clear them** (principle 4). An LLM may find, explain and list
  every open question; only you answer one.
- **Independent readings, and checks from the intent.** The two readers are different models
  that never see the code or each other. Checks and behaviors are written from the chapter, not
  from the code: checks that share a source with the code prove consistency, not intent.

So an LLM can do nearly all the writing. What stays with you:

- what you want, said to the spec partner;
- the answer to every question the readings raise;
- each approval of the oracle: the chapter, the spine, the behaviors, the adapter, the checks and
  the mutants, each a reviewed merge;
- your repository's settings: a protected branch, CI as a required check, and who may push which
  branches.

Use a separate LLM session for each role below, and never the builder's session for any part of
the oracle. Each step has a prompt to start that session, under [Prompts](#prompts).

## 1. Install the toolkit

In the project's root:

```sh
curl -fsSL https://raw.githubusercontent.com/23min/verifier-kit/main/install.sh | bash
```

It writes `verify` at the root and the toolkit in `.verifier-kit/` beside it, from the latest
release, and records which in `.verifier-kit/VERSION`. Commit both, so that CI and the hooks run
the toolkit the project holds. The same line updates them; `... | bash -s -- v0.2.0` installs a
given release. To read the script before it runs, download it first and run it with
`bash install.sh`. It needs Bash, curl, tar, gzip and `git`; `./verify` then needs Go 1.27, and
its first build downloads one Go module. What it writes, refuses and records is in the tool
contract, "Installing" (`docs/reference/tool-contract.md`).

- **An LLM:** an agent can run the line and commit the result.
- **You:** nothing.

## 2. Choose one lifecycle

Pick one thing in the code with a few named states, events that move it between them, and a few
variables with small sets of values, such as a role: an order, an invoice, a document under
review. Every non-terminal state, event and combination of values is one case the spine decides
and `./verify` tries, so keep them few. One lifecycle is one machine; the toolkit checks each
machine alone (README, "Limits").

- **An LLM** reads the code and proposes candidates (prompt [A](#a-finding-a-lifecycle)).
- **You** choose one, and its name: the machine's folder name, such as `order`.

## 3. Write the chapter

`spec/<machine>/chapter.md` says in prose what the lifecycle must do, using only the names in its
glossary. This repo's examples use seven sections, a good shape to start from: 1. Glossary,
2. the process, 3. What must hold, 4. Scenarios, 5. Assumptions, 6. Not covered, 7. What the
lifecycle does not show (`examples/README.md`). What you haven't decided goes in the section it
belongs to, marked "Open question:", not smoothed over.

- **An LLM**, the spec partner, drafts it with you, asks what it needs, and marks its open
  questions (prompt [B](#b-the-spec-partner)).
- **You** say what you want, and approve the chapter by a reviewed merge.
- **Why:** the chapter is your intent; everything after it is checked against it.

## 4. Two readings

Two models each turn the chapter into a spine without seeing the code or each other.

1. Copy an example's reader prompt into `spec/<machine>/readings/prompt.md`, for example notes'
   from the release you installed:
   `https://raw.githubusercontent.com/23min/verifier-kit/v0.1.0/examples/notes/spec/notes/readings/prompt.md`.
   In its rule 1, change `machine: note` to your machine, and say how many variables the
   glossary declares.
2. For each reader, fill the prompt in: replace `{chapter}` with your chapter's text, and
   `{spine_format}` with the spine format's text, from
   `https://raw.githubusercontent.com/23min/verifier-kit/v0.1.0/docs/reference/spine-format.md`.
3. Give each reader only that, and save each reply, unedited, as `reading-a.yaml` and
   `reading-b.yaml`, under a header naming the reader, its model, the date, and the
   `git hash-object` of the chapter, the prompt and the format text it was given, as the
   examples' readings do.

A reader leaves a case it can't decide out, with an `# undecided:` comment.

- **Two LLMs, on different models,** do the reading; each sees only its filled-in prompt.
- **You** start them, or have an agent with access to both models start them.
- **Why:** two independent readings show where the chapter can be read two ways (principle 3);
  readers on the same model tend to share a misreading.

## 5. Find the questions, and answer them

```sh
./verify            # once, to build .verifier-kit/bin/spine; it fails, as nothing is there yet
.verifier-kit/bin/spine diff spec/<machine>/readings/reading-a.yaml spec/<machine>/readings/reading-b.yaml
.verifier-kit/bin/spine check spec/<machine>/readings/reading-a.yaml
.verifier-kit/bin/spine check spec/<machine>/readings/reading-b.yaml
```

Each of these raises questions, and together they find them all:

- `spine diff` gives every case the readings decide differently, with its shortest witness, such
  as "after publish, trash, restore by an editor: A says rejected, B says active".
- `spine check` gives every case a reading left undecided (`undecided-cell`, `missing-case`).
  When both readings leave the same case out, only `spine check` shows it.

Each one is a place where the chapter didn't say what you meant.

- **An LLM** explains each question: what each reading decides, and what each answer would mean.
  It suggests an answer only where the chapter implies one, quoting the text; where the chapter
  is silent, it sets out the options and picks none (prompt [C](#c-the-questions)).
- **You** answer every question, and write each answer back into the chapter, so that the
  chapter and the spine say the same thing. The readings needn't be made again: each keeps
  citing the chapter it was made from. Record the answers where the spine's header can cite
  them: an issue, a commit message, or a file under `spec/<machine>/`.
- **Why:** a silent case is a decision about your intent. A model's suggestion there would be its
  guess at what such systems usually do, accepted under your name (principle 4).

## 6. Ratify the spine

`spec/<machine>/spine.yaml` is a reading with your answers in it. `spine check` must pass: one
rule for every non-terminal state, event and combination of values, and no state unreachable.
Its header says which lifecycle it is, that it was built from `readings/` and your answers,
where those are, and "Ratified by the reviewed merge of this file." The format is in
`docs/reference/spine-format.md`.

- **An LLM** writes it from the answers, gets `spine check` to pass, and shows you `spine diff`
  of the spine against each reading (prompt [D](#d-the-spine)). Those diffs report findings, and
  exit 1: each one should be a case one of your answers decided. It also writes
  `spec/<machine>/drawing.yaml`, where each state sits in the spine's drawing
  (`docs/reference/drawing-layout.md`), and draws `spec/<machine>/drawing.svg` with `spine draw`
  until it gives no warnings.
- **You** review that case-by-case diff and the drawing, and merge the spine with its drawing.
  The merge is the ratification.
- **Why the drawing:** the YAML shows one rule at a time; the drawing shows the machine, its moves
  as arrows and every other rule in a table, each rule once. It is made from the spine, so it
  can't say something else: `./verify` fails while the committed drawing is not what `spine draw`
  makes. Without a `drawing.yaml`, `./verify` skips the step.

## 7. Behaviors, if you want them

Runs you agree on, one YAML file each: `spec/<machine>/behaviors/valid/` must be allowed and end
where they say, `invalid/` must be refused. `./verify` replays them against the spine, so a later
change to the spine that breaks one shows. The format is in `docs/reference/spine-format.md`,
"Steps, examples and behaviors".

- **An LLM** drafts them from the chapter's scenarios, without the code (prompt
  [E](#e-behaviors)).
- **You** review them.

## 8. The adapter

The adapter is how `./verify` drives your code. It is a program that speaks JSON lines on stdin
and stdout (the tool contract, "The adapter protocol"): to `new` it starts a fresh instance, to
`apply` it turns one spine event into calls on the code and says `ok` or `rejected`, and to
`observe` it names the spine state the code is in. `conformance/adapter_<machine>.cmd` holds the
one line that starts it.

- **In Python** it is a file with three functions, `new()`, `apply()` and `observe()`, served by
  the toolkit's shim: `env PYTHONPATH=src adapter_shim.py conformance/adapter_<machine>.py`, as
  `examples/notes/` does. Python writes `__pycache__/` folders beside the files it runs: add
  `__pycache__/` to your `.gitignore`.
- **In any other language** it is a program speaking the protocol itself, as
  `examples/elixir_job/conformance/adapter_job.exs` does.

What it must do, because `conform`'s guarantee rests on it (`docs/design.md`, "What conform
guarantees"):

- **Only translate.** It never decides whether an action is allowed; the code does, and the
  adapter reports what the code did.
- **Observe the truth.** `observe` names the state from the code's real data, not from what the
  adapter remembers. What the adapter can't see, `conform` can't check.
- **Start afresh.** `new` starts from nothing every time, and the code must be deterministic: the
  same events give the same results.
- **State its contract.** Its header says what `src/` must offer it, the whole contract between
  the oracle and the code, as the examples' adapters do.

- **An LLM** writes it from the code and the spine, in a session that is not the builder's
  (prompt [F](#f-the-adapter)).
- **You** check that it only translates, and review it like the spine: it is part of the oracle.

**When existing code disagrees with the spine,** `./verify` reports each case where it does, and
each is a task for the builder: the code doesn't do what you want. Until the first build fixes
them all, `./verify` fails on your main branch. Write the checks first (step 9), then have the
builder fix the code (step 12), and only then add mutants (step 10) and turn on the gates
(step 11): mutants can't run while the code fails the oracle, and a gate would block your own
oracle changes.

## 9. Content checks

The spine sees states and permissions; content checks cover what it can't, such as text, totals
and formats. `checks/run.cmd` holds the line that runs them. It must fail when no check ran; in
Python that is `env PYTHONPATH=src run_checks.py checks`, over `test_*.py` files in
`checks/<machine>/`. Each check cites the cell it depends on, `# covers: <state>/<event>@<hash>`,
with the hash from `.verifier-kit/bin/spine hash spec/<machine>/spine.yaml`; when that cell
changes, `./verify` reports the check as stale (`docs/reference/spine-format.md`, "Citing a
cell").

The spine says nothing about events after a terminal state, so neither `conform` nor the
behaviors check what the code does then. A rule such as "a closed ticket takes no replies"
needs a content check.

- **An LLM** writes them from the chapter, reading the code only for the names it must call, and
  shows that each one fails while the code doesn't yet do what it checks, or names the mutant
  that will show it (prompt [G](#g-content-checks)).
- **You** review them.
- **Why:** a check written from the code repeats what the code does and can't fail for the right
  reason (principle 3); a check that never fails proves nothing (principle 6).

## 10. Mutants

`mutation/mutants.yaml` lists planted bugs: each a name, a file under `src/`, text that occurs
exactly once in it, and what replaces it (the tool contract, `./verify`). `./verify --full` makes
each one in a copy and requires the oracle to catch it, once `./verify` passes on the code as it
is. A mutant that survives is a gap in the oracle.

- **An LLM** writes them from the code, one for each rule worth breaking, runs
  `./verify --full`, and reports any that survive (prompt [H](#h-mutants)).
- **You** review them, and decide what a surviving mutant means: a missing check, or a rule the
  spine doesn't hold.

## 11. The gates

Copy what you use from the toolkit's `integrations/`, at the release you installed, from
`https://raw.githubusercontent.com/23min/verifier-kit/v0.1.0/integrations/<path>`. Each file's
header says how to use it:

- **CI:** `integrations/ci/github-actions.yml` as `.github/workflows/verify.yml`, or
  `integrations/ci/gitlab-ci.yml` as `.gitlab-ci.yml`. It runs `./verify --full` and the path
  guard on pull requests. Set up the languages your pieces need where its comment says.
- **Claude Code:** `integrations/claude-code/hooks/stop-verify.sh` at that same path, and
  `integrations/claude-code/settings.json` as `.claude/settings.json`: deny rules keep the
  agent's file tools out of the oracle, and a Stop hook runs `./verify` whenever the builder
  tries to finish.
- **Git:** `integrations/git-hooks/pre-commit` at that same path, turned on with
  `git config core.hooksPath integrations/git-hooks`, for fast checks before each commit.
- **Review:** `integrations/CODEOWNERS` as `.github/CODEOWNERS` on GitHub, or at the root on
  GitLab, so that people approve every change to the oracle.
- **The path guard:** a `policy.yaml` at the root maps branch names to roles and each role to the
  paths it may change (the tool contract, "The policy file"). For example:

  ```yaml
  branches:
    "build/*": builder
    "*": person
  roles:
    builder:
      allow: ["src/*"]
      deny: ["spec/*", "checks/*", "conformance/*", "mutation/*", ".verifier-kit/*", "verify",
             "policy.yaml", "integrations/*", ".claude/*", ".github/*"]
    person:
      allow: ["*"]
  ```

Files copied from those links lose their execute bit: run `chmod +x` on both hook scripts, and
commit them that way.

- **An LLM** copies the files and writes `policy.yaml` and `CODEOWNERS` for your layout.
- **You** make CI a required check on a protected branch, name the owners in `CODEOWNERS`, and
  let the builder push only to `build/*` branches: the path guard trusts the branch name.
- **Why:** only a required check is a gate; the hooks make failures cheap to fix early, and each
  can be bypassed (README, "Jacking it in").

## 12. Build

The builder works on a `build/*` branch, made for it, and changes `src/` until `./verify` passes.
It can read the oracle but not change it. When it tries to finish, the Stop hook runs `./verify`
and sends it back with the failing lines; after five attempts (`VERIFY_MAX_ATTEMPTS`) it stops
for you. CI runs `./verify --full` on its pull request.

- **An LLM**, the builder, does the work (prompt [I](#i-the-builder)).
- **You** make its branch, take over when the Stop hook gives up, and merge when CI passes. A
  mutant that survives on the builder's pull request is a gap in the oracle, not in the code:
  close it on a branch of your own (steps 9 and 10), and the builder's branch picks it up.
- **After the merge,** add mutants for the rules the code now enforces (step 10).

## 13. When the chapter changes

1. Change the chapter with the spec partner (step 3).
2. Have two new readings made (step 4).
3. Compare each with the ratified spine, to see what the edit means, and answer the questions
   (step 5):
   `.verifier-kit/bin/spine diff spec/<machine>/readings/reading-a.yaml spec/<machine>/spine.yaml`.
4. Ratify the changed spine (step 6). `./verify` reports the drawing as stale until `spine draw`
   writes it again; the layout changes only when a new state needs a place or a label is in the
   way.
5. `./verify` now reports every check that cites a changed cell as stale: rewrite or re-cite them
   with the new hashes (step 9), and add mutants for new rules (step 10).
6. Build (step 12).

## Prompts

Each prompt starts one LLM session for one role. Replace `<machine>` with your machine's name.
A session that needs the format or the protocol reads it from GitHub, at the release you
installed (`.verifier-kit/VERSION` names it), for example:

- the spine format:
  `https://raw.githubusercontent.com/23min/verifier-kit/v0.1.0/docs/reference/spine-format.md`
- the tool contract:
  `https://raw.githubusercontent.com/23min/verifier-kit/v0.1.0/docs/reference/tool-contract.md`
- the drawing layout, in releases from v0.2.0, the first that has `spine draw`:
  `https://raw.githubusercontent.com/23min/verifier-kit/v0.2.0/docs/reference/drawing-layout.md`

The readers (step 4) use the examples' prompt instead.

### A. Finding a lifecycle

```text
Read this codebase and list up to five lifecycles that verifier-kit could check: each a thing
with a few named states, events that move it between them, and at most a few variables with
small sets of values, such as a role. For each, give its states, events and variables, the case
count (non-terminal states times events times value combinations), and where in the code it
lives. Change nothing.
```

### B. The spec partner

```text
You are my spec partner for one lifecycle, <machine>. Help me write spec/<machine>/chapter.md:
what it must do, in plain prose, with these sections: 1. Glossary (every state, event and
variable with its values; the rest of the chapter uses only these names), 2. the process,
3. What must hold, 4. Scenarios, 5. Assumptions, 6. Not covered, 7. What the lifecycle does not
show. A section with nothing to say says "None." Ask me what you need. Where I haven't decided
something, write it in the section it belongs to, marked "Open question:"; never fill a gap from
what such systems usually do. You may read the code for its vocabulary, but the chapter says
what should happen, and where the code seems to do otherwise, ask me.
```

### C. The questions

```text
Below are spec/<machine>/chapter.md, the output of spine diff between two readings of it, and
the output of spine check on each reading. Number the questions: each difference spine diff
reports, and each case spine check reports undecided. For each: say in one line what each
reading decides, using its witness, or that it decides nothing. If the chapter implies an
answer, suggest it and quote the sentence that implies it. If the chapter is silent, list the
possible answers and what each would mean, and pick none. Don't read the code, and don't answer
for me.
```

### D. The spine

```text
Write spec/<machine>/spine.yaml from spec/<machine>/readings/reading-a.yaml and my answers below,
one per question. Change only the cases my answers decide. Head it with the lifecycle it is,
that it was built from readings/ and my answers, where the answers are recorded, and "Ratified
by the reviewed merge of this file." Run .verifier-kit/bin/spine check on it until it reports no
errors, then show me .verifier-kit/bin/spine diff of the spine against each reading. Then write
or update spec/<machine>/drawing.yaml, where each state sits in the drawing (the drawing layout
format), and run .verifier-kit/bin/spine draw spec/<machine>/spine.yaml
spec/<machine>/drawing.yaml > spec/<machine>/drawing.svg, moving states and labels until it gives
no warnings. Don't read the code.
```

### E. Behaviors

```text
From the Scenarios in spec/<machine>/chapter.md, write one behavior file for each scenario under
spec/<machine>/behaviors/: valid/ for runs the chapter allows, invalid/ for runs it forbids, in
the format the spine format's "Steps, examples and behaviors" gives. Don't read the code. Run
./verify and change only your own files until its behaviors step passes.
```

### F. The adapter

```text
Write the adapter for spec/<machine>/spine.yaml: conformance/adapter_<machine>.cmd, one line that
starts a program speaking the adapter protocol (the tool contract, "The adapter protocol"), and
that program. It turns each spine event into calls on the code in src/, says ok or rejected from
what the code did, and names the spine state from the code's real data. It never decides whether
an action is allowed. new() starts from nothing every time. Its header states what src/ must
offer it. Read src/, but don't change it. Run ./verify, fix only the adapter until what fails is
a real disagreement between the code and the spine, and report those.
```

### G. Content checks

```text
Write content checks for what spec/<machine>/spine.yaml can't see, such as text, values and
formats, and what the code does after a terminal state, from spec/<machine>/chapter.md:
checks/<machine>/, and checks/run.cmd, one line that runs them and fails when none ran. Each
check cites the cell it depends on with a comment, covers: <state>/<event>@<hash>, the hash from
.verifier-kit/bin/spine hash spec/<machine>/spine.yaml. Write them from the chapter; read src/
only for the names you must call. For each check, show that it fails while the code doesn't do
what it checks, or name the mutant that will show it. Don't change src/.
```

### H. Mutants

```text
Write mutation/mutants.yaml: for each rule in spec/<machine>/spine.yaml that the code enforces,
and for each content check, one mutant that breaks it: a name, a file under src/, text that
occurs exactly once in that file, and what replaces it. Run ./verify --full and report each
mutant that survives. Don't change anything else.
```

### I. The builder

```text
<the task>. You are on the branch build/<task>; change only src/. ./verify decides when you are
done: when you try to finish, it runs and sends you back with what fails. The spec, the checks,
the adapter, the mutants and the toolkit are fixed. If the spec looks wrong, stop and say so
instead of working around it.
```
