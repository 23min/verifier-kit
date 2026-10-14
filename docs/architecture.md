# Architecture

How the repo fits together today: who calls what, what `./verify` runs, how the examples are
wired, and how the toolkit checks itself. The rules each piece follows are in
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

The first example, in `examples/notes/`; the paths below are relative to it. Its files
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

## The ref_sync example

In `examples/ref_sync/`; the paths below are relative to it. Its spine is one operation's
lifecycle, which `spine conform` checks through the client's pure `transition()`. Two clients
racing, lost replies and crashes are checked by its own explorer, and Git's assumed behavior by
checks against real Git. The adapter's docstring is the whole contract between the oracle and
`src/`.

```mermaid
flowchart LR
  chapter["spec/ref_sync/chapter.md<br/>rules, S1-S7, C01-C12"]
  readings["spec/ref_sync/readings/*.yaml<br/>two readings"]
  spineY["spec/ref_sync/spine.yaml<br/>one operation's lifecycle"]
  behav["spec/ref_sync/behaviors/<br/>valid/ and invalid/"]
  adapter["conformance/adapter_ref_sync.py<br/>new, apply, observe"]
  client["src/ref_sync_client.py<br/>transition, Client, GitRemote"]
  proto["checks/ref_sync/test_protocol.py<br/>C01-C08"]
  explore["checks/ref_sync/explore.py<br/>breadth-first search"]
  world["checks/ref_sync/world.py<br/>simulated remote, faults, S1-S5, S7"]
  gitc["checks/ref_sync/test_git_ref.py<br/>C09-C11, S6"]
  git[("git program<br/>throwaway repositories")]
  mutate["mutation/mutate_ref_sync.py"]
  chapter -->|"read by two interpreters"| readings
  readings -->|"spine diff; a person ratifies"| spineY
  behav -->|"spine behaviors replays them"| spineY
  spineY -->|"spine conform: every sequence up to 4 events"| adapter
  adapter -->|"calls transition"| client
  proto --> explore
  proto --> world
  world -->|"reads the states"| spineY
  world -->|"steps two Clients against a simulated remote"| client
  gitc -->|"publishes with GitRemote"| client
  gitc --> git
  client -->|"GitRemote runs"| git
  mutate -->|"an edited copy shadows it"| client
```

The explorer is generic: `explore.py` knows nothing of Git, and `world.py` holds the moves, the
faults and the invariants. `git` is a dependency of this example only; without it the Git
checks fail.

## Checking the checker

`./verify --full` runs each example's mutation script. `examples/notes/mutation/mutate_notes.py`
breaks the service on purpose and expects the oracle to notice. It runs from the example's
folder and finds `tools/` above it. `examples/ref_sync/mutation/mutate_ref_sync.py` works the
same way with six edits to `src/ref_sync_client.py`, and names every check that failed.

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
exit code and the lines it must print. The faults are all in the notes example, but `./verify`
in the copy checks every example, so ref_sync's checks run in each scenario too.

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
| 9 | the notes service imports a module that doesn't exist | `./verify examples/notes` | `adapter-failed`, `check-unloadable` |

This repo's CI (`.github/workflows/verify.yml`) runs `./verify --full`, then
`demo/faults.sh`, on every pull request and push to `main`. It does not set `VERIFY_BASE`, so
the path guard runs here only inside scenario 4.

## Per machine, per root and repo-wide

| Part | Scope today | How it finds its inputs |
|---|---|---|
| `spine check`, `behaviors`, `stale`, `conform` | per machine | `{root}/spec/{m}/`, `{root}/checks/{m}/`, `{root}/conformance/adapter_{m}.py` |
| content checks | per root: one run over all of `{root}/checks/` | every `test_*.py` under `{root}/checks/` |
| mutation | per root: every `{root}/mutation/mutate_*.py` under `--full` | file name pattern; each script runs all of its root's `checks/` |
| `src/` | per root, on `PYTHONPATH` | module name (`notes_app`, `ref_sync_client`) |
| path guard | repo-wide | `policy.yaml` patterns over `examples/*/` and the toolkit's folders |
| deny rules, CODEOWNERS (`integrations/`) | a project's root | folder patterns for a project with `spec/` at its root |
| `demo/faults.sh` | seeds faults in notes only; its `./verify` runs every example | hard-coded `examples/notes/` paths |
| `demo/live-builder.sh` | notes only | hard-coded `examples/notes/` paths |
| choosing what to run | one root per call | `./verify ROOT`; without it, the discovery rule above |
