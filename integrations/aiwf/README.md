# aiwf as one more caller (example)

Nothing in this repo needs aiwf. If a project already uses aiwf, its contracts can call
`spine` the same way CI does. This snippet comes from an earlier demo that was run against
aiwf at commit 9db8775; it is not exercised by this repo.
`tools/bin/spine` is there once `./verify` has run, or after
`go -C tools build -o bin/spine ./cmd/spine`.

```yaml
# aiwf.yaml
contracts:
  validators:
    spine-trace:                       # one behavior file per call
      command: tools/bin/spine
      args: [trace, "{{schema}}", "{{fixture}}"]
  entries:
    - id: C-0001
      validator: spine-trace
      schema: spec/notes/spine.yaml
      fixtures: spec/notes/behaviors   # aiwf wants <fixtures>/<version>/valid|invalid
```

Two differences from this repo's layout:

- aiwf expects a version folder under the fixtures (`spec/notes/behaviors/v1/valid/...`);
  `spine behaviors` reads `valid/` and `invalid/` directly.
- With a single valid fixture, aiwf can report a rejected fixture as a validator error.
  Keep at least two valid fixtures per version.

Conformance can run as a contract too (validator `spine conform` with the fixture as the
adapter), but `./verify` already runs it; one caller per check keeps the two from drifting.
