"""Two ref-sync clients against a simulated remote: scenarios C01-C08 of the chapter.

Each scenario explores every interleaving of the clients' steps, and the faults it allows,
up to a bound, and checks S1-S5 and S7 in every world it reaches (world.problems). Where
every operation has ended, it also checks what the chapter says must happen. A failure names
the shortest sequence of moves that shows it. A pass means "no problem within this bound".
"""
# covers: spine@23fc1b40ab2e  (the scenarios follow the lifecycle, and their outcomes name its states)
import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import world as W                      # noqa: E402
from explore import explore             # noqa: E402

MAX_MOVES = 30


def outcomes(w):
    return sorted(w.journals[n][i["event_id"]]["state"] for n, its in w.submitted.items() for i in its)


def run(scenario, title, intents, want, needs=None, **faults):
    """Explore one scenario; want(world) says what is wrong in a world where every operation ended."""
    start = W.World(intents, **faults)

    def problems(w):
        wrong = W.problems(w)
        if not wrong and W.done(w):
            wrong = want(w)
        return wrong

    res = explore(start, W.moves, W.play, problems, MAX_MOVES)
    ended = [w for w, _ in res.worlds if W.done(w)]
    cut = sum(1 for w, trace in res.worlds if len(trace) == MAX_MOVES and not W.done(w))
    reach = (f"stopped at {MAX_MOVES} moves with {cut} runs unfinished" if cut
             else "every reachable world")
    print(f"info    explored           {scenario}: {title}; {start.bound()}; {reach}: "
          f"{res.states} worlds, {len(ended)} with every operation ended")
    assert res.problem is None, f"{scenario}: {res.problem}; after: {' -> '.join(res.trace) or 'the start'}"
    assert ended, f"{scenario}: no operation sequence ended within {MAX_MOVES} moves"
    if needs:
        assert any(needs(w) for w in ended), f"{scenario}: no run within the bound shows it"
    return res


def both(state):
    return lambda w: [] if outcomes(w) == [state, state] else [f"expected both {state}, got {outcomes(w)}"]


def one_each(a, b):
    return lambda w: [] if outcomes(w) == sorted([a, b]) else [f"expected one {a} and one {b}, got {outcomes(w)}"]


ALICE_X = W.make_intent("e-alice", "alice", "x", "a")
BOB_Y = W.make_intent("e-bob", "bob", "y", "b")
BOB_X = W.make_intent("e-bob", "bob", "x", "b")
BOB_Y_SAME_ID = W.make_intent("e-alice", "bob", "y", "b")


def test_c01_different_entities_both_publish():
    run("C01", "Alice and Bob change different entities from the same tip",
        {"alice": [ALICE_X], "bob": [BOB_Y]}, both("confirmed"),
        needs=lambda w: w.remote.counts["refused"] > 0)


def test_c02_same_entity_one_conflicts():
    run("C02", "Alice and Bob change the same entity, expecting the same version",
        {"alice": [ALICE_X], "bob": [BOB_X]}, one_each("confirmed", "conflicted"),
        needs=lambda w: w.remote.counts["refused"] > 0)


def test_c03_reply_lost_after_publication():
    run("C03", "a push is published but its reply is lost",
        {"alice": [ALICE_X], "bob": [BOB_Y]}, both("confirmed"),
        needs=lambda w: w.remote.counts["lost after delivery"] > 0, lost_replies=1)


def test_c04_push_lost_before_publication_then_other_publishes():
    run("C04", "a push is lost or delayed before publication, different entities",
        {"alice": [ALICE_X], "bob": [BOB_Y]}, both("confirmed"),
        needs=lambda w: w.remote.counts["delayed"] > 0, lost_replies=1, delayed=True)
    run("C04", "a push is lost or delayed before publication, same entity",
        {"alice": [ALICE_X], "bob": [BOB_X]}, one_each("confirmed", "conflicted"),
        needs=lambda w: w.remote.counts["delayed"] > 0, lost_replies=1, delayed=True)


def test_c05_crash_keeps_the_intent():
    run("C05", "clients crash and start again, with a lost reply",
        {"alice": [ALICE_X], "bob": [BOB_Y]}, both("confirmed"),
        needs=lambda w: sum(w.crashes.values()) > 0, lost_replies=1, delayed=True, crashes=1)


def test_c06_same_event_id_other_digest():
    run("C06", "Bob's intent has Alice's event ID but another digest",
        {"alice": [ALICE_X], "bob": [BOB_Y_SAME_ID]}, one_each("confirmed", "protocol_error"))


def _broken(after_files, state="queued"):
    """A world whose remote holds one commit made by hand, for checking the checks."""
    w = W.World({"alice": [ALICE_X]})
    r = w.remote
    if after_files:
        r.deliver(r.tip, dict(r.commits[r.tip][1], **after_files), False)
    w.journals["alice"][ALICE_X["event_id"]]["state"] = state
    return W.problems(w)


def test_c07_event_without_its_change_is_caught():
    event_only = _broken({"events/e-alice.json": json.dumps(ALICE_X)})
    change_only = _broken({"entities/x.json": json.dumps({"value": "a", "version": 1})})
    assert any(p.startswith("S4") for p in event_only), event_only
    assert any(p.startswith("S4") for p in change_only), change_only


def test_c08_confirmed_before_published_is_caught():
    wrong = _broken({}, state="confirmed")
    assert any(p.startswith("S5") for p in wrong), wrong
