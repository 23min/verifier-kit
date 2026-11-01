"""apply() returns a text close to what the spine expects, the next of four at each call (every
sequence diverges at its first event), so each shows in a divergence."""

VALUES = ["crashed (no)", "ok ", "OK", ""]
count = [0]


def new():
    return None


def apply(door, event, env):
    count[0] += 1
    return VALUES[count[0] % len(VALUES)]


def observe(door):
    return "closed"
