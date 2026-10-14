"""Connects the spine to the ref-sync client.

Owned by the oracle side: the builder can read it but not change it.
It only translates. It never decides whether an event is allowed.

  new()      -> a fresh operation: a client submits one intent; reads its state from the journal
  apply()    -> hands one spine event to the client's lifecycle; returns "ok" or "rejected"
  observe()  -> the operation's state, as the client's lifecycle last returned it

What src/ must offer, in a module named ref_sync_client. The checks in checks/ref_sync/ use
the same names, so this is the whole contract between the oracle and the client.

Data (all JSON-compatible):

  intent    {"event_id", "actor", "entity", "expected_version", "value", "digest"}, made by the
            caller, digest included. The client never changes an intent or recomputes its digest.
  files     the complete tree of a workflow-ref commit, path -> JSON text:
              entities/<entity>.json   {"value": ..., "version": <int>}
              events/<event_id>.json   the intent, unchanged
            The first commit holds entities x and y, each {"value": null, "version": 0}.
  journal   a dict the caller passes in and keeps across crashes. The client keeps one record
            per intent under its event ID, {"intent": <intent>, "state": <spine state>}, from
            submit() on, and updates "state" whenever the operation's state changes.

The lifecycle, a pure function:

  transition(state, event) -> the next state, or None when the event is refused.
            States and events are the spine's names (spec/ref_sync/spine.yaml).

The remote, which the caller passes in (checks/ref_sync/world.py has the simulated one):

  remote.fetch() -> (tip, files)      the workflow ref's commit ID and its tree
  remote.push(parent, files, force=False) -> "accepted" or "non_fast_forward"
            asks the remote to move the workflow ref to a new commit with that parent and tree.
            Raises TimeoutError when the push ends without a definite answer.

The client:

  Client(name, remote, journal)
            starts, or starts again after a crash, from the journal: every operation that is
            not final goes on as queued (the spine's restart). Neither this nor submit() calls
            the remote.
  client.submit(intent)
            writes {"intent": intent, "state": "queued"} under the intent's event ID.
  client.step() -> True if it did something, False if every operation is final
            takes one action for the oldest operation that is not final, calling the remote
            at most once. A crash can come between any two steps, so everything the next steps
            depend on after a restart must already be in the journal.

The Git-backed remote, used by checks/ref_sync/test_git_ref.py:

  GitRemote(repo, url)
            the same fetch() and push() against the workflow ref refs/workflow/state of the
            Git repository at url, using the local repository at repo for its objects. It
            leaves that repository's HEAD, index, working tree and branches as they were.

Constraints, so the checks can explore every interleaving: the client is synchronous and
deterministic (no threads, sleeps, clocks or unseeded randomness), and its objects hold only
plain data (dicts, lists, tuples, sets, strings, numbers, booleans, None and plain objects of
those) besides the remote and the journal it was given, so they can be copied and compared.
The client imports nothing from conformance/ or checks/.
"""
import ref_sync_client

INTENT = {"event_id": "e1", "actor": "alice", "entity": "x", "expected_version": 0,
          "value": "a", "digest": "d1"}


class _NoRemote:
    """Stands in for the remote where the contract says the client calls none."""

    def fetch(self):
        raise AssertionError("the client called the remote while starting or submitting")

    def push(self, parent, files, force=False):
        raise AssertionError("the client called the remote while starting or submitting")


def new():
    journal = {}
    ref_sync_client.Client("conform", _NoRemote(), journal).submit(dict(INTENT))
    return {"state": journal[INTENT["event_id"]]["state"]}


def apply(h, event, params):
    nxt = ref_sync_client.transition(h["state"], event)
    if nxt is None:
        return "rejected"
    h["state"] = nxt
    return "ok"


def observe(h):
    return h["state"]
