"""door_outputs.py whose outputs() raises (#116): the shim can't say what a step sent, so it stops
with the reason, and conform reports the adapter failed."""

from door_outputs import new, apply, observe  # noqa: F401


def outputs(door):
    return {"sent": door.sent}["what was sent"]
