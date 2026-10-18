"""apply() returns a different kind of value for each case, so each shows in a divergence."""
import math

VALUES = [True, None, 1e16, 1e-5, -0.0, 10 ** 30, math.nan, math.inf, 0.1, 123456789.0,
          "crashed (no)", "", "it's", [], {}, {"k": "v'\""}]
count = [0]


def new():
    return None


def apply(door, event, env):
    count[0] += 1
    return VALUES[count[0] % len(VALUES)]


def observe(door):
    return "closed"
