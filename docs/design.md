# Design: verifier-kit

This is the normative design for the toolkit, taken from the v3 design page
(11 October 2026). From now on this file is the current truth: change it here, in the same
pull request as the code it describes. The v3 page is background and history.

The toolkit serves a way of working with coding agents. People own a small, executable oracle
that states what a system must do. A builder agent owns only the code. Deterministic tools
decide whether the two agree, and they run from hooks and CI, so nothing depends on an agent
choosing to check its own work.

The toolkit itself is ordinary software. Its own acceptance test is the notes example: `./verify`
must pass on it, and `demo/faults.sh` must catch every seeded fault.

## Principles

1. **Acceptance is deterministic, generation is not.** A model's output varies; a fixed check
   doesn't. The oracle is the fixed reference signal in a control loop.
2. **Judgment lives in executable checks.** Prose decays and can't be run; approval of
   generated code is lost when the code is regenerated. Checks survive regeneration.
3. **The builder never touches the oracle.** Independent readings make the gaps between intent
   and code visible; that is where loom found its value on real code (loom
   `work/decisions/D-0006-loom-loop-qualified-proceed-to-build-the-thin-tool.md`).
   Readers can share a misreading, so they should differ: other models, separate contexts. And
   checks that share a source with the code prove consistency, not intent.
4. **Models may raise doubts, never clear them.** Only a command settles a claim: expected
   result, observed output, environment.
5. **One place per fact.** Normative docs describe the system as it is now; proposals,
   decisions and audits carry a date and may age.
6. **Check the checker.** A new check must fail before the code is written; planted bugs must
   be caught; a check that never catches anything is a candidate for removal.
7. **Smallest oracle that does the job.** When an oracle nears the size of the code it checks,
   switch to properties and examples.
8. **Same process at every size.** Small projects test the mechanics; long-running ones show
   drift.
9. **Nothing is skipped silently.** A check that does not run says so and why; a missing piece
   is an error, not a pass. Green means "as far as the oracle measures", so the output must
   show how far that was.

## Decisions taken

- **No workflow tool underneath.** The ledger is the tracker (GitHub issues here, GitLab issues
  or Jira at work), git history, CI and code owners. aiwf, where used, is one more caller of
  the same tools through its contracts.
- **CI is the orchestrator.** There is no stateful controller. Ordering is enforced by required
  checks, active steps are CI jobs triggered by events, and each run derives the current state
  from the repo and the tracker. A `status` command will print an overview that can always be
  regenerated.
- **One entry point.** Agent hooks, git hooks and CI all call `./verify` and nothing else.

## Documents

| Kind | Holds | Ages? |
|---|---|---|
| Change proposal (an issue or epic) | what is about to change and why | yes: closed and kept for the reasons |
| Normative docs (chapters, architecture, glossary) | how the system behaves now | no: current truth |
| Decision records (ADRs) | one decision, context, consequences | superseded rather than edited |
| The oracle | spine, decision tables, behaviors, checks, adapter, dependency rules | no: must match the normative docs |
| Observational (audits, measurements) | what was found on a given day | yes: dated, with the command that reproduces it |

A change proposal is a delta against the normative chapters. When it is ratified, the delta is
merged into the chapters, the oracle is updated to match, and the proposal is closed.

## The four loops

### Intent loop (slow, you lead)

You and a spec partner (an LLM) write the change, chapter by chapter: intent, behavior written
with glossary names only, what not to build, likely changes, and open questions stated rather
than smoothed over. The glossary is a closed vocabulary; a name outside it is itself a finding.
Ratification is a reviewed merge with code owners on `spec/`. Chapters need not be complete
before building; the next loop finds what's missing.

### Interpretation loop (finds ambiguity, blocks nothing)

Two interpreters on different models, with separate contexts and no access to the code, each
turn the changed sections into a spine. Each gets a fixed bundle (section, glossary, spine
schema) and two instructions: decide every case, and write `undecided: "<what is missing>"`
instead of guessing. `spine diff` compares the readings case by case and by behavior, and
each disagreement becomes an issue with its shortest witness, phrased as a question. Your
answers become the ratified spine. The same diff of a new reading against the current spine
shows what a chapter edit means.

Limits: two readings can share a misreading; only formalizable content is covered.

### Build loop (fast, automated)

The builder works on a `build/*` branch, may change `src/` and little else, reads the oracle,
and never sees holdout scenarios. Its own unit tests are welcome but don't count toward the
gate. A Stop hook runs `./verify` when the builder tries to finish and sends back the failing
lines; after a set number of attempts it hands over to a person. CI is the gate.

The builder needs a dispute channel: a file it may write that makes CI fail with "open
dispute" until a person resolves it, which then becomes an issue.

### Drift loop (keeps docs true, advisory)

Most doc drift is designed out first:

| What the doc says | How it stays true | Blocks a merge? |
|---|---|---|
| Facts the code knows (command lists, codes, config keys) | generated between markers; CI fails if stale | yes |
| Examples and commands | run in CI | yes |
| References (links, ids, code symbols) | resolved by a script | yes |
| Rules the oracle states | rendered from the spine, not restated | yes |
| Claims about behavior and design | the doc reader, below | no |
| Reasons and history | dated, allowed to age | no |

The doc reader: code decides when to read (sections declare what they cover; a change to
covered code flags them, and CI requires an edit or a "still true" stamp; a scheduled sweep
reads the oldest first). The reader may only raise doubts, each with a probe (command, what the
doc says, what it outputs). A person decides which side is wrong. Claims that keep mattering
become checks.

For the spine: it records the hash of the chapter section it came from, and a one-sided change
fails without a cited decision. On any spine change CI posts the case-level diff for review. A
surface check fails on public operations the adapter neither maps nor lists as out of scope.
Later: replay production events through the spine (process-mining conformance checking).

## The oracle

A diagram that can't fail a build is documentation, not oracle.

### The spine

One ratified state machine per feature, covering lifecycle and permissions only. Format and
checks: `docs/reference/spine-format.md`. Precedent: SCR's tables with automated checks for
missing and overlapping cases.

### The other views

| View | Guards | Form | Status |
|---|---|---|---|
| Behaviors | intent at the edges | valid and invalid event sequences | built |
| Content checks | data the spine can't see | tests citing spine cells | built |
| Decision tables | pure computations | inputs to output, checked for completeness | designed |
| Invariants and properties | laws, per-state invariants, metamorphic relations | property-based tests | reuse a library |
| Reference model | a complex part | a naive, obviously correct implementation | reuse |
| Sequence rules | order across components | rules marked may, must or never, checked on traces | designed |
| Dependency rules | layers and boundaries | ArchUnit, import-linter, dependency-cruiser | reuse |
| Contracts and schemas | boundaries | OpenAPI or JSON Schema with valid and invalid fixtures | reuse |
| Budgets | latency, cost, size | fitness functions in CI | reuse |
| Holdout scenarios | intent, end to end | scenarios the builder never sees | designed |

Failure behavior (timeouts, retries) is written as events in the spine. Concurrency needs a
model checker or deterministic simulation.

Views are kept consistent through the spine: checks cite cells by hash, decision tables hang on
transitions, sequence rules and the spine judge the same trace.

### The adapter

`new()`, `apply()`, `observe()`; translates, never decides; owned by the oracle side; observes
real side effects. An adapter runs as a program speaking the same three operations as JSON
lines over stdin and stdout (`docs/adr/0003-adapter-protocol.md`); a Python file is served that
way by `tools/adapter_shim.py` (`docs/adr/0004-go-spine.md`), and an adapter in any other
language is its own program. Details in `docs/reference/tool-contract.md`.

### Checking the checker

Red first; mutation (planted bugs must be caught); a positive control (a known-good
implementation must pass); seeded faults across the loop (`demo/faults.sh`); retirement of
checks that never catch anything.

## Code the oracle can check

Guidance for whoever writes the system under test:

- **Functional core, imperative shell:** decisions in pure functions, I/O in a thin shell. The
  biggest lever.
- **Ports and adapters:** the app runs without UI or database, so the adapter attaches at a port.
- **Observable state:** the adapter can name the spine state from real data.
- **Deterministic seams:** clock, randomness and ids are injected; a flaky check counts as failing.
- **Dependency rules from day one.**
- **Illegal states unrepresentable** in types and schemas.
- **Facts as events** where the lifecycle changes often.
- **Failures as events** in the spine.

## Changing the oracle

Blast radius, cheapest first: a guard changes; a transition is added; a state is added; a state
is removed, merged or split; the model is reframed. The last two need a migration table that
maps every old state to a new one, and the gate checks it is total. Changing a fact state needs
a decision; a policy change cites one; a design change needs none while facts and policy hold.
`spine stale` marks every check that cites a changed cell.

## One change, end to end

1. A proposal changes chapter sections on a `spec/*` branch.
2. CI runs `interpret` on the changed sections, then `spine diff`.
3. Disagreements become issues with witnesses; the change waits for answers.
4. The spine change is reviewed with its case-level diff; code owners approve.
5. Checks for new or changed cells are added and confirmed to fail on the current code.
6. The builder runs on a `build/*` branch; the Stop hook runs `./verify`.
7. CI runs `./verify --full` with the path guard; a pass merges and the proposal closes.
8. The drift loop runs on merges and on a schedule.

## Gates

| Layer | What it does | Bypassed by |
|---|---|---|
| Agent harness | deny rules on oracle paths; Stop hook | shell commands that write files (add the OS sandbox) |
| Git | fast checks before each commit | `--no-verify` |
| CI | full checks plus the path guard | admin override only, once required on a protected branch |
| Review | code owners on the oracle and normative docs | GitLab Free has no code-owner approval; the guard stands in |

GitHub private repositories need a paid plan for required checks and code-owner review. GitLab
"Pipelines must succeed" and protected branches work on Free; code-owner approval needs Premium.

## Measuring the loop

Seeded faults per loop; disagreement rate per section; escaped defects; mutation score and
retired checks; minutes per ratification and tokens per change; check growth against code
growth; "still true" stamps later contradicted by an audit.

## Status at the start of this repo

| Part | Status |
|---|---|
| Spine format, `check`, `diff`, `hash`, `stale`, `behaviors`, `trace`, `conform`, `guard` | built |
| `./verify`, gate examples for GitHub, GitLab, Claude Code, git hooks, code owners | built; GitLab example not yet run |
| Mutation (hand-picked), seeded faults | built |
| Builder loop with Stop hook | run live with Claude Code |
| `interpret` | designed |
| Decision tables, sequence rules, holdout | designed |
| Docs checks, doc reader | designed |
| Source link, review diff, surface check | designed |
| Migration tables, dispute channel, `status` command | designed |
| Adapter protocol for other languages, with a Python shim; `./verify` runs adapters, checks and mutation as commands | built |
| SARIF | designed |
| Production replay | later |

From here on, progress is tracked in GitHub issues, not in this table.

## Where to be skeptical

- A spine that is complete and wrong passes every check; ratification is the only check on intent.
- Gaming is real but narrower than often assumed. loom's pre-registered tests found no
  spec-weakening when an agent was graded on its own implementation (three fresh subjects, all
  NO-GO; loom
  `work/decisions/D-0002-build-loom-light-does-the-gaming-effect-re-validate-on-fresh-subjects.md`
  and
  `work/decisions/D-0005-reallocate-two-dimension-verdict-no-go-neither-failure-mode-reproduced.md`),
  but did see agents over-claim what they had shown. Agents have also been seen editing or
  deleting tests to make them pass (ImpossibleBench). The path guard and deny rules are there
  for that; read-only access and holdouts reduce it.
- Two interpreters can share a misreading.
- The best-known successes had an oracle for free; business rules rarely do.
- Specs as practiced (linked tickets) haven't been shown to reduce defects; executable checks
  are the untested bet.
- Upkeep cost can outpace value.
- Tacit knowledge can't all be written down; too many constraints make change harder.
- Green means "as far as the oracle measures".

## Open decisions

- Who writes the oracle's other views: a person, or a third agent whose output is ratified.
- The interpret bundle: models, prompt, every section or a sample.
- Agreement for decision tables: generated inputs and equality.
- The dispute format.
- The retirement rule for checks.

## Relation to loom

[loom](https://github.com/23min/loom) checks a short set of formal claims against modules written
for a verifier (Dafny proofs, or the TLC model checker) and reports what it could not establish
as gaps. verifier-kit runs the real code against an intent model, up to a bounded number of
events (`spine conform`). They are complementary. loom links to real code only through
hand-written examples; its gap G-0020
(`work/gaps/G-0020-no-tooling-derives-host-language-tests-from-verified-claims.md`) asks for
tests derived from verified claims and run against the real implementation.

## Key sources

- Bornholt et al., lightweight formal methods for S3 ShardStore (SOSP '21)
- Heitmeyer, applying the SCR requirements method
- Parnas and Clements, A Rational Design Process: How and Why to Fake It
- Knight and Leveson, and the 2026 replication with coding agents (N-version programming)
- ImpossibleBench (ICLR 2026)
- Böckeler, Understanding Spec-Driven Development (Kiro, spec-kit, Tessl)
- OpenSpec concepts (specs, changes, archive)
