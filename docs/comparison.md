# verifier-kit and other tools

This page is for readers who know proof assistants, model checkers, model-based or
property-based testing, or spec-driven agent tools, and want to know where verifier-kit stands
among them. The facts about other tools were read from their own documentation, papers and
release pages on 15 November 2026; each section links its sources. If something here is out of
date, please open an issue.

## What verifier-kit establishes

verifier-kit checks **real code, in any language**, against a **finite, deterministic state
machine** that a person owns: the spine, with exactly one rule for every state, event and
combination of a few finite variables ([introduction](introduction.md)).

- **What it decides.** `spine check` decides that every case of the spine has exactly one rule
  and every state is reachable. That is a complete check of the spine itself, since the spine is
  finite.
- **What it tests.** `spine conform` drives the code through an adapter, from a shortest path to
  each state, with every event and value, and K further steps (the margin, 1 by default). When
  every case agrees, every implementation with up to K states that the spine doesn't have is
  caught (Vasilevskii 1973, Chow 1978). That holds under stated assumptions: `new` starts afresh,
  `observe` reads the code's real state, and the code is deterministic. State that only a
  refusal changes is out of reach at any margin ([`design.md`, "What conform
  guarantees"](design.md#what-conform-guarantees)).
- **What it isn't.** It is testing with a stated bound, not proof. It runs one lifecycle at a
  time, so it says nothing about races. It sees states and permissions, not data; content
  checks cover data, and mutants check that the oracle notices planted bugs.
- **Who owns it.** The spine is made from your prose by two independent readings, with each
  disagreement put to you as a question, and you ratify it. Hooks, a required CI check and a path
  guard keep the builder agent from changing it.

## In short

| Family | Tools | Establishes | On | Better than verifier-kit at | verifier-kit adds |
|---|---|---|---|---|---|
| Proof assistants and verified languages | Lean 4, Rocq, Dafny, Verus | a machine-checked proof for every input | code written in, or translated into, their language | proof, data and computation, no assumption about hidden state | any language without translating the code; a far cheaper spec, made from prose |
| Model checkers | TLA+, Quint, Alloy, P | every state of a bounded model, or proof | a model; code only through traces that happen to occur | concurrency, failures, liveness, rich data | runs chosen to reach every case of the real code |
| Model-based and stateful testing | XState's `@xstate/test`, GraphWalker, Hypothesis stateful, QuickCheck state machines | every path of the model, or random runs, with coverage measured | the real code | data in events, long random runs, races (parallel tests), UI | a stated bound on what is missed; a model ratified from prose; gates |
| Property-based testing | Hypothesis, QuickCheck | random inputs, shrunk to the smallest failure | the real code | content and computation, cheaply | every case of a lifecycle, not a sample |
| Spec-driven agent tools | GitHub Spec Kit, Kiro | an agent's judgment; Kiro adds solver-checked requirements and random property tests | the spec, and the code through the agent | the whole path from idea to tasks; any kind of requirement | an oracle the builder can't write or change, judged by a program |

## Proof assistants and verified languages

[Lean 4](https://lean-lang.org/) and [Rocq](https://rocq-prover.org/) (formerly Coq) are proof
assistants: a small trusted kernel checks every proof. [Dafny](https://dafny.org/) and
[Verus](https://verus-lang.github.io/verus/guide/) are verified languages: an SMT-based verifier
proves that the code meets its pre- and postconditions and invariants.

- **What they prove.** A property for every input and every run length, data included, with no
  assumption about hidden state. Nothing here comes close. The proof is only as good as the
  theorem stated: one benchmark's postcondition turned out to be stated wrongly
  ([Harmonic](https://harmonic.fun/news/verina-benchmark/)).
- **On what.** Code written in their language, or translated into it. Dafny compiles to C#, Go,
  Python, Java and JavaScript; Rocq extracts to OCaml or Haskell; Verus checks a subset of Rust;
  Rust reaches Lean through translation pipelines
  ([arXiv:2605.30106](https://arxiv.org/abs/2605.30106)).
- **AI.** Provers are getting much better at the proofs:
  - Off-the-shelf LLMs succeeded on 82% of Dafny tasks, 44% of Verus and 27% of Lean in the
    vericoding benchmark ([arXiv:2509.22908](https://arxiv.org/abs/2509.22908), September 2025).
  - AutoVerus produced correct proofs for over 90% of 150 tasks
    ([arXiv:2409.13082](https://arxiv.org/abs/2409.13082)).
  - Harmonic's Aristotle proved or disproved 96.8% of the VERINA benchmark's problems, with the
    specs given.

  The specs remain someone's job, and that is where verifier-kit works.
- **What verifier-kit adds.** Code in any language, checked where it is, with no translation. A
  spec small enough to make from prose, with its ambiguities put to you as questions. Gates that keep the builder from changing it.
- **Together.** Prove a pure core, a computation or a decision, in Lean or Dafny, and let conform
  check the lifecycle of the code around it. AWS's Cedar keeps a Lean model of its Rust code and
  tests the two against each other ([arXiv:2407.01688](https://arxiv.org/abs/2407.01688)), which
  has the shape of conform. Proving properties of one spine adds little: it is finite, and
  `spine check` already decides it.

## Model checkers and specification languages

- [TLA+](https://foundation.tlapl.us/) models systems, especially concurrent and distributed
  ones. TLC enumerates every state of a model within its constants, Apalache checks bounded runs
  or inductive invariants with SMT, and TLAPS proves.
- [Quint](https://quint.sh/docs/model-based-testing) is a typed language on the same logic, with
  a simulator and the same two model checkers behind it.
- [Alloy](https://alloytools.org/about.html) explores relational models within a bounded scope.
- [P](https://p-org.github.io/P/) models systems as communicating state machines and explores
  their schedules; AWS uses it widely.

- **What they establish.** Every state of a bounded model, or a proof (TLAPS, inductive
  invariants). Their strength is what a spine can't say: interleavings, failures, liveness and
  temporal properties, and data.
- **On what.** A model. Several now reach code through traces:
  - TLA+ trace validation checks traces recorded from a real program
    ([arXiv:2404.16075](https://arxiv.org/abs/2404.16075)), and Microsoft's CCF runs it in CI.
  - [Quint Connect](https://quint.sh/posts/quint_connect) replays the model's random runs against
    Rust code.
  - P's [PObserve](https://p-org.github.io/P/advanced/pobserve/pobserve/) checks production logs.

  These check the runs that tests, random simulation or production happen to produce. Conform
  chooses runs that reach every case of the spine, and says what it can miss.
- **AI.** The TLA+ extension for VS Code has an MCP server that runs TLC. Quint Connect was
  designed for LLMs to write its glue code, and P's PeasyAI generates models from design
  documents. Writing correct specs is still hard: in one study of 30 LLMs, 8.6% of their TLA+
  specs were semantically correct ([arXiv:2606.05792](https://arxiv.org/abs/2606.05792)).
- **What verifier-kit adds.** The real code, in any language, tried in every case of its
  lifecycle, with no model language to learn. A spec checked for completeness the moment it is
  written.
- **Together.** Model the protocol between clients in TLA+, Quint or P, and check it there; give
  each entity's lifecycle a spine, and check the code with conform. The ref_sync example's
  interleaving explorer does by hand, in its own checks, what a model checker does in general.

## Model-based and stateful testing

These are verifier-kit's closest relatives: they too run a model and the real code side by
side.

- [`@xstate/test`](https://stately.ai/docs/xstate-test) drives the code from an XState machine
  through a small adapter, and compares after every step. Version 2, in beta for XState 6 and
  built on fast-check, can run every path of the machine's graph, and fail CI below full
  transition coverage. The npm package's 0.5 line is deprecated.
- [GraphWalker](https://graphwalker.github.io/) generates paths through a graph model, at random
  or by a covering route, until a coverage goal is met. The Java project is archived in favour of
  a Rust rewrite.
- [Hypothesis's stateful testing](https://hypothesis.readthedocs.io/en/latest/stateful.html) and
  QuickCheck's state machines ([PropEr](https://proper-testing.github.io/apidocs/proper_statem.html),
  Erlang QuickCheck,
  [quickcheck-state-machine](https://hackage.haskell.org/package/quickcheck-state-machine))
  generate random command sequences from a model written in code, and shrink a failure to its
  shortest form.

- **What they establish.** Coverage, or a random sample, with no stated bound on what is
  missed: "full transition coverage means every declared transition was taken at least once, not
  that every behavior was tested" (`@xstate/test`).
- **Where they do better.** Data in events, with generators and shrinking. Long random runs,
  which can reach hidden state beyond a fixed margin. Races on the real code: PropEr's
  `parallel_commands` and quickcheck-state-machine look for a serial order that explains what
  concurrent calls returned. User interfaces, through Playwright.
- **What verifier-kit adds.** Every case tried, with a stated bound: an implementation with up to
  K hidden states beyond the spine's can't pass. A model ratified from prose through independent
  readings, rather than written as test code. Any language, through an adapter that speaks JSON
  lines. Gates that keep the builder out, and mutants that check the checks.
- **Together.** A parallel state-machine test, run as a content check, covers races, for which
  verifier-kit has no general check. GraphWalker or `@xstate/test` can walk a user interface over
  a service whose lifecycle conform checks.

## Property-based testing

[Hypothesis](https://hypothesis.readthedocs.io/en/latest/) and
[QuickCheck](https://hackage.haskell.org/package/QuickCheck) generate inputs at random to falsify
a stated property, and shrink a failure to its smallest form. Agents can write them:
[Anthropic's agentic property-based testing](https://www.anthropic.com/research/property-based-testing)
ran on more than 100 Python packages and reported bugs, about half of those reviewed valid.

- **Where they do better.** Content and computation, over unbounded data, cheaply. A spine can't
  express "the total is the sum of the lines".
- **What verifier-kit adds.** Every case of a lifecycle tried, not a sample, and a person's
  oracle that the builder can't rewrite.
- **Together.** verifier-kit's design already counts properties as a view of the oracle
  ([`design.md`, "The other views"](design.md#the-other-views)): a Hypothesis suite can be the
  content checks that `checks/run.cmd` runs, each test citing the spine cell it depends on.

## Spec-driven agent tools

- [GitHub Spec Kit](https://github.com/github/spec-kit) runs a process of agent commands, from
  constitution and specification to plan, tasks and implementation. `/speckit.analyze` checks
  that spec, plan and tasks agree before implementation, without looking at code. Since June
  2026, `/speckit.converge` has the agent assess the code against the spec and add unfinished work
  as tasks. Tests are optional.
- [Kiro](https://kiro.dev/docs/specs/) works from requirements in EARS notation.
  - Its [requirements analysis](https://kiro.dev/blog/deep-spec-analysis/) (May 2026) has an LLM
    turn each requirement into logic that SMT solvers check for contradictions and gaps. It puts
    each ambiguity to you as a question with two options, close to what `spine check` and
    `spine diff` do.
  - Its [property-based tests](https://kiro.dev/docs/specs/correctness/) run random cases
    against the code.

- **What they establish.** In Spec Kit, an agent's judgment. In Kiro, a solver's check of an
  LLM's formalization, which is made afresh each run and not kept for you to ratify, and random
  tests that "provide evidence of correctness, not a proof".
- **Where they do better.** The whole path from idea to tasks, for any kind of requirement, not
  only lifecycles, and with little to learn.
- **What verifier-kit adds.** An oracle that a person ratifies and keeps, and that the builder
  can neither write nor change. When one of Kiro's property tests fails, Kiro can offer to fix
  "the spec, implementation, or test itself"; we found no documented mechanism that keeps the
  agent from changing what judges it. With verifier-kit, a program judges, and the hooks and CI
  make sure it does.
- **Together.** Spec Kit's `spec.md` can serve as the chapter. Kiro's requirements analysis can
  check the chapter before the two readings, and its property tests can become content checks,
  kept on the oracle's side.
