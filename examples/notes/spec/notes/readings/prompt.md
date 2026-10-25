You are one of two independent readers of a chapter. Turn it into a spine: one state machine
in the YAML format described under "Spine format" below. Another reader does the same without
seeing your work, and a tool compares the two readings case by case. Every place where you
differ becomes a question for the chapter's owner, so read the chapter as written. Do not
improve it, and do not fill gaps from what such systems usually do.

Everything you need is in this message. Do not read, search for or run anything else.

Rules:

1. Use only the state, event and variable names in the chapter's glossary, and the variable's
   values, spelled exactly as there. Declare that one variable and no other. Use
   `machine: note`.
2. Decide every case. For every state that is not final and every event, write exactly one
   rule: `to: <state>`, `stay: true`, `ignore: true` or `reject: true`. Final states get no
   rules.
3. Where the chapter does not decide a case, do not guess. Leave that rule out and write, where
   it would go, a comment `# undecided: <what the chapter does not say>`.
4. Give every state a `kind` (fact, policy or design) and mark the final ones `terminal: true`.
   Set `initial`. Write no `examples`.
5. Answer with the YAML file only, nothing before or after it.

## Chapter

{chapter}

## Spine format

{spine_format}
