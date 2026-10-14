"""Breadth-first search over the worlds a deterministic system can reach.

Generic: nothing here knows about Git or ref-sync. A caller gives a starting world and three
functions: the moves possible in a world, how to play one move on a world (returning a
description of what happened, or None when the move does nothing), and what is wrong with a
world. Every move is tried on a copy of every world reached, up to a number of moves. Worlds
with the same fingerprint count once. The search stops at the first world with something
wrong; since the search is breadth-first, the moves that led there are a shortest witness.
"""
import copy
import hashlib
from collections import deque


class Result:
    def __init__(self):
        self.states = 0        # distinct worlds reached
        self.problem = None    # the first thing found wrong, if any
        self.trace = ()        # the moves that reached it
        self.worlds = []       # (world, trace) for every world reached, in search order


def explore(start, moves, play, problems, max_moves):
    result = Result()
    seen = {fingerprint(start)}
    queue = deque([(start, ())])
    while queue:
        world, trace = queue.popleft()
        result.states += 1
        result.worlds.append((world, trace))
        wrong = problems(world)
        if wrong:
            result.problem, result.trace = wrong[0], trace
            return result
        if len(trace) == max_moves:
            continue
        for move in moves(world):
            nxt = copy.deepcopy(world)
            what = play(nxt, move)
            if what is None:
                continue
            key = fingerprint(nxt)
            if key not in seen:
                seen.add(key)
                queue.append((nxt, trace + (what,)))
    return result


def fingerprint(obj):
    """A hash of a world's contents that does not depend on object identity or set order."""
    parts, memo = [], {}

    def walk(o):
        if o is None or isinstance(o, (bool, int, float, str)):
            parts.append(repr(o))
            return
        if id(o) in memo:
            parts.append(f"@{memo[id(o)]}")
            return
        memo[id(o)] = len(memo)
        if isinstance(o, dict):
            items(o.items())
        elif isinstance(o, (list, tuple)):
            parts.append("[" if isinstance(o, list) else "(")
            for x in o:
                walk(x)
                parts.append(",")
            parts.append("]")
        elif isinstance(o, (set, frozenset)):
            parts.append("<" + ",".join(sorted(fingerprint(x) for x in o)) + ">")
        elif hasattr(o, "__dict__") or hasattr(type(o), "__slots__"):
            fields = list(vars(o).items()) if hasattr(o, "__dict__") else []
            fields += [(n, getattr(o, n)) for n in getattr(type(o), "__slots__", ()) if hasattr(o, n)]
            parts.append(type(o).__module__ + "." + type(o).__qualname__)
            items(fields)
        else:
            raise TypeError(f"cannot compare worlds holding a {type(o).__name__}; "
                            "keep only plain data in them")

    def items(pairs):            # not memoized: only objects that live in the world are
        parts.append("{")
        for k, v in pairs:
            walk(k)
            parts.append(":")
            walk(v)
            parts.append(",")
        parts.append("}")

    walk(obj)
    return hashlib.sha256("".join(parts).encode()).hexdigest()
