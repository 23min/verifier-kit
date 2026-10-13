# Architecture

How the repo fits together today: who calls what, what `./verify` runs, how the notes example
is wired, and how the toolkit checks itself. The rules each piece follows are in
`docs/reference/`; the reasons are in `docs/design.md`. In the diagrams, `{m}` is a machine's
folder name, such as `notes`, and `{root}` is the folder `./verify` checks: a project laid out
with `spec/`, `checks/`, `conformance/`, `src/` and `mutation/`. In this repo each
`examples/<name>/` is a root; a project that uses the toolkit is its own root.

## Layers

Callers know about `./verify`. `./verify` knows the folder layout. The tools know only the
files they are given.

```mermaid
flowchart LR
  subgraph callers["Callers"]
    stop["Claude Code Stop hook<br/>integrations/claude-code/hooks/stop-verify.sh"]
    pre["git pre-commit<br/>integrations/git-hooks/pre-commit"]
    ci["CI job<br/>.github/workflows/verify.yml"]
    faults["demo/faults.sh"]
    person["a person"]
  end
  verify["./verify<br/>finds the pieces by folder name"]
  subgraph tools["Tools: files in, findings out"]
    spine["tools/spine.py<br/>check, behaviors, stale, conform, guard"]
    rc["tools/run_checks.py"]
  end
  mut["{root}/mutation/mutate_*.py"]
  subgraph files["Files in the repo"]
    oracle["the oracle, per root<br/>spec/, checks/, conformance/"]
    code["the code, per root<br/>src/"]
    policy["policy.yaml"]
  end
  stop -->|"./verify, fast checks"| verify
  pre -->|"./verify"| verify
  ci -->|"./verify --full"| verify
  faults -->|"./verify, on a broken copy"| verify
  person --> verify
  verify --> spine
  verify --> rc
  verify -->|"--full only"| mut
  mut -->|"runs as subprocesses"| spine
  mut --> rc
  spine --> oracle
  spine --> code
  spine --> policy
  rc --> oracle
  rc --> code
```

## What `./verify` runs

```mermaid
flowchart TD
  start(["./verify [ROOT] [--full]"]) --> roots{"ROOT given?"}
  roots -- yes --> one["that folder"]
  roots -- no --> has{"spec/ here, or<br/>no examples/*/?"}
  has -- yes --> here["this folder"]
  has -- no --> each["every examples/*/, sorted"]
  one --> g
  here --> g
  each --> g
  g{"VERIFY_BASE set?"} -- yes --> guard["spine guard policy.yaml<br/>files changed since VERIFY_BASE<br/>once, whatever the roots"]
  g -- no --> skipg["skip guard"]
  guard --> rootloop
  skipg --> rootloop
  subgraph rootloop["once per root"]
    subgraph loop["once per {root}/spec/{m}/spine.yaml"]
      check["spine check"] -- sound --> beh["spine behaviors<br/>spec/{m}/behaviors/"]
      check -- not sound --> skipm["skip the machine's other steps"]
      beh --> stale["spine stale<br/>checks/{m}/"]
      stale --> conf["spine conform<br/>conformance/adapter_{m}.py<br/>PYTHONPATH={root}/src:{root}/conformance"]
    end
    loop --> content["content checks: run_checks.py {root}/checks --require-checks<br/>PYTHONPATH={root}/src"]
    content --> f{"--full?"}
    f -- yes --> mut["every {root}/mutation/mutate_*.py"]
    f -- no --> skipmut["skip mutation"]
  end
  rootloop --> sum["one summary line; exit 0 or 1"]
```

A step that cannot run because a piece is missing is an error, not a skip, reported per root:
no spine at all (`no-machines`), no adapter for a machine (`no-adapter`), no `test_*` function
anywhere under the root's `checks/` (`no-checks`), or no mutation script under `--full`
(`no-mutation`). A root other than the repo itself starts with a `== <root>` line.

## The notes example

The only example today, in `examples/notes/`; the paths below are relative to it. Its files
are tied together by the name `notes`, by Python imports, and by the cell hashes the checks
cite.

```mermaid
flowchart LR
  chapter["spec/notes/chapter.md<br/>prose"]
  readings["spec/notes/readings/*.yaml<br/>two readings"]
  spineY["spec/notes/spine.yaml<br/>ratified spine"]
  behav["spec/notes/behaviors/<br/>valid/ and invalid/"]
  adapter["conformance/adapter_notes.py<br/>new, apply, observe"]
  app["src/notes_app.py<br/>the service"]
  checks["checks/notes/test_content.py"]
  mutate["mutation/mutate_notes.py"]
  chapter -->|"read by two interpreters"| readings
  readings -->|"spine diff; a person ratifies"| spineY
  behav -->|"spine behaviors replays them"| spineY
  spineY -->|"spine conform: every sequence up to 4 events"| adapter
  adapter -->|"imports and calls"| app
  checks -->|"imports"| app
  checks -.->|"cite cells as covers: state/event@hash"| spineY
  mutate -->|"an edited copy shadows it"| app
```

Who may change what, from `policy.yaml` (enforced by `spine guard` when `VERIFY_BASE` is set):

| Branch | Role | May change | May not change |
|---|---|---|---|
| `build/*` | builder | `examples/*/src/`, `docs/`, `README.md` | `examples/*/spec/`, `examples/*/checks/`, `examples/*/conformance/`, `examples/*/mutation/`, `tools/`, `integrations/`, `verify`, `policy.yaml`, `.claude/`, `.github/`, `.gitlab/`, CODEOWNERS |
| `spec/*` | spec-author | `examples/*/spec/`, `examples/*/checks/`, `examples/*/conformance/` | `examples/*/src/` |
| anything else | person | everything | |

## Checking the checker

`./verify --full` runs `examples/notes/mutation/mutate_notes.py`, which breaks the service on
purpose and expects the oracle to notice. It runs from the example's folder and finds `tools/`
above it.

```mermaid
sequenceDiagram
  participant M as mutate_notes.py
  participant T as temp folder
  participant C as spine conform (notes)
  participant R as run_checks.py checks
  loop each of 5 hand-picked edits to src/notes_app.py
    M->>T: write notes_app.py with one edit
    M->>C: run with PYTHONPATH=temp:src:conformance
    M->>R: run over the example's checks/, same PYTHONPATH
    Note over M: killed if either exits 1
  end
  Note over M: exits 1 if any mutant survived
```

## Acceptance: `demo/faults.sh` and CI

`demo/faults.sh` is the toolkit's own acceptance test. Each scenario makes a fresh copy of the
repo (without `.git`) in a temp folder, seeds one fault, runs a command there, and checks the
exit code and the lines it must print.

```mermaid
flowchart LR
  repo["working copy"] -->|"tar, without .git"| copy["temp copy"]
  copy --> seed["seed one fault"]
  seed --> run["run ./verify, spine diff,<br/>or the Stop hook"]
  run --> expect{"expected exit code<br/>and expected lines?"}
  expect -- yes --> ok["scenario ok"]
  expect -- no --> fail["scenario fails; faults.sh exits 1"]
```

| # | Fault seeded | Command | Must print |
|---|---|---|---|
| 0 | none | `./verify --full` | `all steps passed` |
| 1 | the service as first written | `./verify` | two `diverges` lines with their sequences |
| 2 | the notes spine lets editors restore | `./verify` | `behavior-allowed`, `stale-check`, `diverges` |
| 3 | `on:` instead of `event:` in the notes spine | `./verify` | `yaml-boolean-key`, and the notes steps skipped |
| 4 | a `build/*` branch edits the notes spine | `VERIFY_BASE=main ./verify` | `path-denied examples/notes/spec/notes/spine.yaml` |
| 5 | none (two readings) | `spine diff` on the notes readings | two `case-disagrees` lines |
| 6 | notes content checks deleted | `./verify --full` | `mutant-survived` for the upper-case mutant |
| 7 | the service as first written | the Stop hook, twice | a block, then the attempt-cap warning |
| 8 | the notes adapter, content checks and mutation removed | `./verify --full` | `no-adapter`, `no-checks`, `no-mutation` |

This repo's CI (`.github/workflows/verify.yml`) runs `./verify --full`, then
`demo/faults.sh`, on every pull request and push to `main`. It does not set `VERIFY_BASE`, so
the path guard runs here only inside scenario 4.

## Per machine, per root and repo-wide

| Part | Scope today | How it finds its inputs |
|---|---|---|
| `spine check`, `behaviors`, `stale`, `conform` | per machine | `{root}/spec/{m}/`, `{root}/checks/{m}/`, `{root}/conformance/adapter_{m}.py` |
| content checks | per root: one run over all of `{root}/checks/` | every `test_*.py` under `{root}/checks/` |
| mutation | per root: every `{root}/mutation/mutate_*.py` under `--full` | file name pattern; `mutate_notes.py` itself runs all of its root's `checks/` |
| `src/` | per root, on `PYTHONPATH` | module name (`notes_app`) |
| path guard | repo-wide | `policy.yaml` patterns over `examples/*/` and the toolkit's folders |
| deny rules, CODEOWNERS (`integrations/`) | a project's root | folder patterns for a project with `spec/` at its root |
| `demo/faults.sh`, `demo/live-builder.sh` | notes only | hard-coded `examples/notes/` paths |
| choosing what to run | one root per call | `./verify ROOT`; without it, the discovery rule above |
