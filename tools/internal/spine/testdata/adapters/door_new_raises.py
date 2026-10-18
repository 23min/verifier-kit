"""new() raises: adapter-failed, and nothing compares."""

def new():
    raise RuntimeError("no database")


def apply(door, event, env):
    return "ok"


def observe(door):
    return "closed"
