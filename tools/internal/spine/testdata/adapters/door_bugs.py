"""seeds/spine.yaml's door with bugs: guests may open it, removing an open door removes it, and
it answers "done" where "ok" is expected."""
import sys


def new():
    return {"state": "closed"}


def apply(door, event, env):
    print("apply", event, env, file=sys.stderr)
    print("printing to stdout goes to stderr through the shim")
    if event == "open" and door["state"] == "closed":
        door["state"] = "open"
        return "ok"
    if event == "remove":
        door["state"] = "gone"
        return "ok"
    return "done"


def observe(door):
    return door["state"]
