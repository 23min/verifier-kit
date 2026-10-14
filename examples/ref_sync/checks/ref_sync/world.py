"""The world around ref-sync clients, and what must hold in it (chapter.md, S1-S5 and S7).

Oracle side. A world is one simulated remote, some clients from src/ref_sync_client.py, the
journal each client was given, and the intents submitted to them. The remote behaves as
chapter section 5 assumes Git does: it moves the workflow ref only for a fast-forward, unless
the push is forced, as a default bare repository allows. It can lose a push's reply after
delivering the push, or hold the push back and deliver it later. A crash replaces a client
with a new one built from the same journal. checks/ref_sync/test_git_ref.py checks the
remote's assumed behavior against real Git.

explore.py searches the moves below; problems() says what is wrong in a world.
"""
import hashlib
import json
from pathlib import Path

import yaml

import ref_sync_client

SPINE = yaml.safe_load((Path(__file__).resolve().parents[2] / "spec/ref_sync/spine.yaml").read_text())
STATES = set(SPINE["states"])
FINAL = {s for s, meta in SPINE["states"].items() if (meta or {}).get("terminal")}
ENTITIES = ("x", "y")
FIRST_TREE = {f"entities/{e}.json": json.dumps({"value": None, "version": 0}) for e in ENTITIES}


def make_intent(event_id, actor, entity, value, expected_version=0):
    body = {"event_id": event_id, "actor": actor, "entity": entity,
            "expected_version": expected_version, "value": value}
    digest = hashlib.sha256(json.dumps(body, sort_keys=True).encode()).hexdigest()[:12]
    return dict(body, digest=digest)


def commit_id(parent, files):
    return hashlib.sha256(json.dumps([parent, sorted(files.items())]).encode()).hexdigest()[:12]


class SimRemote:
    def __init__(self):
        root = commit_id(None, FIRST_TREE)
        self.commits = {root: (None, dict(FIRST_TREE))}
        self.tip = root
        self.tips = [root]       # every commit the workflow ref has pointed to, in order
        self.held = []           # delayed pushes not yet delivered: (client, parent, files, force)
        self.counts = {"refused": 0, "lost after delivery": 0, "delayed": 0}
        self.fault = None        # set by the world for the next push only
        self.caller = None
        self.calls = []          # what the current move did, for the trace
        self.broken = None       # set when a push names a parent the remote does not have

    def fetch(self):
        self.calls.append("fetches")
        return self.tip, dict(self.commits[self.tip][1])

    def push(self, parent, files, force=False):
        fault, self.fault = self.fault, None
        if fault == "delayed":
            self.held.append((self.caller, parent, dict(files), bool(force)))
            self.counts["delayed"] += 1
            self.calls.append("pushes; the push is delayed and the reply lost")
            raise TimeoutError("push delayed, reply lost")
        result = self.deliver(parent, files, force)
        self.calls.append(f"pushes ({result}{', forced' if force else ''})")
        if fault == "lost after delivery":
            self.counts["lost after delivery"] += 1
            self.calls[-1] += "; the reply is lost"
            raise TimeoutError("reply lost")
        return result

    def deliver(self, parent, files, force):
        if parent not in self.commits:
            self.broken = f"a push names parent {parent!r}, which the remote does not have"
            return "non_fast_forward"
        if parent != self.tip and not force:
            self.counts["refused"] += 1
            return "non_fast_forward"
        cid = commit_id(parent, files)
        self.commits[cid] = (parent, dict(files))
        self.tip = cid
        self.tips.append(cid)
        return "accepted"


class World:
    def __init__(self, intents, lost_replies=0, delayed=False, crashes=0):
        """intents: client name -> list of intents to submit at the start."""
        self.remote = SimRemote()
        self.journals = {name: {} for name in intents}
        self.clients = {}
        self.submitted = {name: [dict(i) for i in its] for name, its in intents.items()}
        self.limits = {"lost replies": lost_replies, "delayed": delayed, "crashes": crashes}
        self.lost = 0
        self.crashes = {name: 0 for name in intents}
        self.error = None
        for name in sorted(intents):
            self.clients[name] = ref_sync_client.Client(name, self.remote, self.journals[name])
            for intent in intents[name]:
                self.clients[name].submit(dict(intent))

    def bound(self):
        lost, crashes = self.limits["lost replies"], self.limits["crashes"]
        return (f"{len(self.clients)} clients, {sum(map(len, self.submitted.values()))} intents, "
                f"{_count(lost, 'lost reply', 'lost replies')}"
                f"{' (a push may also be delayed)' if lost and self.limits['delayed'] else ''}, "
                f"{_count(crashes, 'crash', 'crashes')}{' per client' if crashes else ''}")


def _count(n, one, many):
    return f"no {many}" if n == 0 else f"{n} {one if n == 1 else many}"


def moves(w):
    out = []
    faults = []
    if w.lost < w.limits["lost replies"]:
        faults = ["lost after delivery"] + (["delayed"] if w.limits["delayed"] else [])
    for name in sorted(w.clients):
        out.append(("step", name, None))
        out += [("step", name, f) for f in faults]
        if w.crashes[name] < w.limits["crashes"]:
            out.append(("crash", name))
    out += [("deliver", i) for i in range(len(w.remote.held))]
    return out


def play(w, move):
    r = w.remote
    if move[0] == "deliver":
        caller, parent, files, force = r.held.pop(move[1])
        return f"{caller}'s delayed push arrives ({r.deliver(parent, files, force)})"
    name = move[1]
    if move[0] == "crash":
        w.crashes[name] += 1
        try:
            w.clients[name] = ref_sync_client.Client(name, r, w.journals[name])
        except Exception as e:
            w.error = f"{name} could not start again from its journal: {type(e).__name__}: {e}"
        return f"{name} crashes and starts again {_states(w, name)}"
    fault = move[2]
    r.fault, r.caller, r.calls = fault, name, []
    try:
        did = w.clients[name].step()
    except Exception as e:
        w.error = f"{name}'s step raised {type(e).__name__}: {e}"
        did = True
    pushed = any(c.startswith("pushes") for c in r.calls)
    r.fault = None
    if not did or (fault and not pushed):
        return None                       # nothing happened, or the fault had no push to act on
    if fault:
        w.lost += 1
    if len(r.calls) > 1:
        w.error = f"{name} called the remote {len(r.calls)} times in one step"
    return f"{name} {'; '.join(r.calls) or 'steps without calling the remote'} {_states(w, name)}"


def _states(w, name):
    return "[" + ", ".join(f"{eid}: {rec.get('state')}" for eid, rec in w.journals[name].items()) + "]"


def done(w):
    return all(w.journals[n].get(i["event_id"], {}).get("state") in FINAL
               for n, its in w.submitted.items() for i in its)


def history(remote):
    """Commit IDs from the tip back to the first commit, oldest first."""
    chain, cid = [], remote.tip
    while cid is not None:
        chain.append(cid)
        cid = remote.commits[cid][0]
    return chain[::-1]


def problems(w):
    if w.error or w.remote.broken:
        return [w.error or w.remote.broken]
    r, out = w.remote, []
    chain = history(r)
    for t in r.tips:
        if t not in chain:
            out.append(f"S1: commit {t}, once the tip, is no longer in the workflow ref's history")
    events = {}
    for parent, child in zip(chain, chain[1:]):
        out += _commit_problems(r.commits[parent][1], r.commits[child][1], child, events)
    tree = r.commits[r.tip][1]
    for name, journal in w.journals.items():
        for eid, rec in journal.items():
            if rec.get("state") == "confirmed":
                held = tree.get(f"events/{eid}.json")
                if held is None or json.loads(held).get("digest") != rec["intent"]["digest"]:
                    out.append(f"S5: {name} has {eid} confirmed, but the tip does not hold it "
                               f"with digest {rec['intent']['digest']}")
    for name, intents in w.submitted.items():
        for intent in intents:
            rec = w.journals[name].get(intent["event_id"])
            if rec is None or rec.get("intent") != intent:
                out.append(f"S7: {name}'s journal no longer holds intent {intent['event_id']}")
            elif rec.get("state") not in STATES:
                out.append(f"{name}'s journal records {intent['event_id']} in "
                           f"{rec.get('state')!r}, which is not a spine state")
    return out


def _commit_problems(before, after, cid, events):
    added = sorted(set(after) - set(before))
    removed = sorted(set(before) - set(after))
    changed = sorted(p for p in set(before) & set(after) if before[p] != after[p])
    new_events = [p for p in added if p.startswith("events/")]
    republished = _republished(before, after, cid, changed, new_events, events)
    if republished:
        return republished
    if len(new_events) != 1:
        return [f"S4: commit {cid} adds {len(new_events)} event records, not one"]
    event = json.loads(after[new_events[0]])
    entity = f"entities/{event.get('entity')}.json"
    out = []
    if removed or changed != [entity] or added != new_events:
        out.append(f"S4: commit {cid} adds event {event.get('event_id')} but changes "
                   f"{changed or 'no entity'}, adds {added}, removes {removed}")
    events[event.get("event_id")] = cid
    if entity in before and entity in after:
        old, new = json.loads(before[entity]), json.loads(after[entity])
        if old.get("version") != event.get("expected_version"):
            out.append(f"S3: commit {cid} publishes {event.get('event_id')}, which expects "
                       f"{event.get('entity')} at version {event.get('expected_version')}, "
                       f"on version {old.get('version')}")
        elif new != {"value": event.get("value"), "version": old.get("version", 0) + 1}:
            out.append(f"S4: commit {cid} sets {event.get('entity')} to {new}, which is not "
                       f"what event {event.get('event_id')} says")
    return out


def _republished(before, after, cid, changed, new_events, events):
    """S2: a commit that rewrites a published event's record, or applies a published event again.

    A record lives at events/<event_id>.json, so publishing an event ID a second time rewrites
    that file, or leaves it as it was, rather than adding a second record. A change that matches
    what a published event did, made without a new record, is taken as that event applied again.
    """
    out = []
    for path in changed:
        if path.startswith("events/"):
            old, new = json.loads(before[path]), json.loads(after[path])
            out.append(f"S2: commit {cid} rewrites the record of event {old.get('event_id')}, "
                       f"published in commit {events.get(old.get('event_id'))}, with digest "
                       f"{new.get('digest')} instead of {old.get('digest')}")
    if new_events:
        return out
    for eid, published in events.items():
        event = json.loads(before.get(f"events/{eid}.json", "{}"))
        entity = f"entities/{event.get('entity')}.json"
        if entity in changed:
            old, new = json.loads(before[entity]), json.loads(after[entity])
            if new == {"value": event.get("value"), "version": old.get("version", 0) + 1}:
                out.append(f"S2: commit {cid} applies event {eid}, published in commit "
                           f"{published}, again: it sets {event.get('entity')} to {new} "
                           f"without adding an event record")
    return out
