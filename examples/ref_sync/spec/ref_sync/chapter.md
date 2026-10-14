# ref-sync: publishing workflow events to one Git ref

Two clients record workflow events, such as "entity x now has value 2", in one shared Git ref.
The ref has its own history, apart from the code branches, and is never merged into `main`.
Either client may crash, lose its connection or lose a reply from the remote at any step, and
both may publish at the same moment. This chapter says what must still hold.

## 1. Glossary

The rest of this chapter uses only these names for the states and events of an operation.

Things:

- **client**: a cooperating program run by one person (here Alice or Bob), with its own local
  data. It can stop and start again at any point.
- **remote**: the one Git repository both clients publish to.
- **workflow ref**: `refs/workflow/state` on the remote. Its history has its own root and shares
  no commits with `main`.
- **tip**: the commit the workflow ref points to on the remote.
- **entity**: one record kept in the workflow ref, with a value and a version number. Here there
  are two, `x` and `y`.
- **intent**: one requested change, fixed once made: an event ID, the actor, the entity, the
  version the actor expects the entity to have, and the new value.
- **event ID**: an identifier chosen when the intent is made and kept through every retry.
- **digest**: a hash of the intent's content. The same event ID with the same digest is the same
  intent.
- **journal**: the client's local, durable record of its intents and of how far each has got.
- **candidate**: a commit the client builds on a tip it has fetched. Its tree holds both the
  entity's new value and version and a record of the event.
- **publication**: the remote moving the workflow ref to a candidate. It does so only for a push
  without force whose candidate has the current tip as its parent (a fast-forward).
- **operation**: one intent on its way from the journal to a final outcome. Section 2 gives the
  lifecycle of one operation.

States of an operation:

- **queued**: the intent is in the journal, and the client holds no candidate for it.
- **prepared**: the client has built a candidate and is pushing it.
- **uncertain**: a push ended without a definite answer, so the client does not know whether
  its event was published.
- **confirmed**: the client has seen its event published, with the same digest. Final.
- **conflicted**: the intent no longer fits the published state: the entity is not at the
  version the intent expects. Someone has to make a new intent, with a new event ID. Final.
- **protocol_error**: the remote holds an event with this event ID but a different digest. A
  person has to look into it. Final.

Events of an operation, each something the client observes:

- **prepare**: the client builds a candidate on the tip it fetched last.
- **accepted**: the remote answers a push: it moved the workflow ref to the candidate.
- **non_fast_forward**: the remote answers a push: refused, because the tip is no longer the
  candidate's parent.
- **response_lost**: a push ends without a definite answer: the reply was lost, the connection
  dropped or the client gave up waiting.
- **seen_on_remote**: a fetch shows this event ID in the workflow ref, with the same digest.
- **absent_on_remote**: a fetch shows no event with this event ID.
- **digest_mismatch**: a fetch shows this event ID with a different digest.
- **invalidated**: checking the intent against the fetched state shows the entity is not at the
  version the intent expects.
- **restart**: the client starts again after a crash or a stop, and reads its journal.

## 2. Publishing one event

The client follows these rules, in this order:

1. It writes the intent to its journal before anything else. From then on the intent survives
   any crash until the operation reaches a final state, and the journal records which one.
2. It fetches the workflow ref. If its event is there with the same digest, an earlier push
   landed and the operation is confirmed. If its event ID is there with a different digest, the
   operation ends in protocol_error. This check comes first after every fetch, including the
   first.
3. Otherwise it checks the intent against the fetched state. If the entity is not at the
   version the intent expects, the operation is conflicted. The client never changes the
   expected version to make an intent fit.
4. Otherwise it builds a candidate on the fetched tip and pushes it, without force, to the
   workflow ref.
5. If the remote accepts the push, the operation is confirmed.
6. If the remote refuses the push because the tip has moved, another client published first.
   That is contention, not yet a conflict: the operation goes back to rule 2 with the same
   intent.
7. If the push ends without a definite answer, the operation is uncertain. The client must not
   report success or failure, and must not make a new event ID. It fetches again: if its event
   is there with the same digest, the operation is confirmed; if its event ID is there with a
   different digest, it ends in protocol_error; if its event ID is not there, the operation goes
   back to rule 2 with the same intent.
8. A crash can happen at any point. On restart, every operation in the journal that has not
   reached a final state goes back to rule 2, whatever it was doing. Only the intent was kept:
   any candidate is gone, and so is the knowledge that a push was uncertain.

What the client can observe depends on where the operation is. While it is queued, the client
fetches, checks the intent and builds a candidate, and pushes nothing. While it is prepared, the
client is pushing and observes only the remote's answer. While it is uncertain, the client only
fetches. A restart can happen in any of them. Any other event is refused.

A fetch that fails changes nothing; the client tries again later.

A push that was uncertain may still land later. That is safe. If it lands before the client's
next push, the client finds its event at rule 2. If the next push lands first, the late push is
no longer a fast-forward and the remote refuses it.

## 3. What must hold

- **S1. No lost confirmed events.** Every confirmed event stays in the workflow ref's history
  exactly once, and no later publication replaces it.
- **S2. No duplicate effects.** An event ID changes an entity at most once, even after a lost
  reply and a retry. The same event ID with a different digest never counts as the same event.
- **S3. Checked transitions.** Every published change fitted the entity's version at the point
  where it was published. An intent that expects an old version never overwrites a newer one.
- **S4. State and event together.** Every published change of an entity has its event record,
  and every published event record has its change, in the same commit. No tip holds one without
  the other.
- **S5. No false confirmation.** A client reports confirmed only when its event, with the same
  digest, is published. A time-out alone never establishes success.
- **S6. Code history untouched.** Publishing never changes `refs/heads/main`, never adds a
  commit to a code branch, and never needs the workflow ref merged into `main`.
- **S7. Intents survive crashes.** After any crash, an intent in the journal can be
  recovered until its operation reaches a final state. Losing the whole disk is out of scope.

## 4. Scenarios

| ID | Scenario | What must happen |
|---|---|---|
| C01 | Alice and Bob fetch the same tip and change different entities | The first push is published; the second is refused, fetches again and is published; both events remain |
| C02 | Alice and Bob fetch the same tip and change the same entity, expecting the same version | The first is published; the second ends conflicted, never a silent overwrite |
| C03 | Alice's push is published but the reply is lost | Alice's client fetches, finds the event with the same digest, and confirms; the change happens once |
| C04 | Alice's push is refused or lost before publication, then Bob publishes | Alice retries with the same event ID on Bob's tip, and is published if the intent still fits or ends conflicted |
| C05 | Alice's client crashes with an intent in its journal | On restart it still has the intent and finishes the operation safely |
| C06 | A client uses an event ID that the remote holds with a different digest | The operation ends in protocol_error; it is never treated as already published |
| C07 | A tip holds an event record without its entity change, or the reverse | Must never happen; the checks catch a client that does it |
| C08 | A client reports confirmed before its event is published | Must never happen; the checks catch a client that does it |
| C09 | A push tries to rewrite the workflow ref without a fast-forward | The remote refuses it, and no event disappears |
| C10 | Clients publish while `main` has its own commits | `main` and its history are unchanged; workflow commits appear only on the workflow ref |
| C11 | A fresh clone of the remote | It does not have the workflow ref until it fetches it explicitly; then it sees the published state |
| C12 | The Git host does not accept custom refs | Not run, only documented: a protected branch with its own root, such as `refs/heads/workflow-state`, is the fallback, chosen on purpose and never silently |

## 5. Assumptions

- The remote applies each update of the workflow ref atomically and refuses a push without force
  that is not a fast-forward. Nobody force-pushes to the workflow ref or deletes it.
- Published commits stay reachable; nothing removes the events used to recognize a retry.
- A fetch returns the remote's current tip.
- Clients follow these rules. A client with push access that does not is out of scope.
- Connections can fail and replies can be lost at any step, and after a while connections work
  again.
- The journal survives a crash.

## 6. Not covered

- Any particular workflow tool, Dolt, a hosted Git service or a network.
- Merging into or rebasing `main`.
- How event IDs are allocated; the two entities exist from the start.
- Malicious clients, stolen credentials, force pushes, and whether a Git host protects custom
  refs.
- Authorization, including permissions revoked while an operation is under way.
- A promise that every operation is eventually published, or a proof beyond the bounds the
  checks state.

## 7. What the lifecycle does not show

Section 2 implies a lifecycle for one operation, as one client sees it. Checking a client
against that lifecycle one operation at a time, as `spine conform` does, does not cover two
clients publishing at once, and the lifecycle cannot tell whether an observation such as
seen_on_remote was true. Two clients racing, lost replies and crashes are checked separately:
by running two clients against a simulated remote through every interleaving up to a stated
bound, and by testing the remote's assumed behavior against a real Git repository.
