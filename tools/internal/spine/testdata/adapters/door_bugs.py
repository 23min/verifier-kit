"""seeds/spine.yaml's door with bugs: guests may open it, removing an open door removes it, and
some answers are not text, so their str() shows in the findings."""
import sys

calls = [0]                 # module state lasts across sequences, as it does in one process


def new():
    return {"state": "closed"}


def apply(door, event, env):
    calls[0] += 1
    print("apply", event, env, file=sys.stderr)
    print("printing to stdout goes to stderr through the shim")
    if event == "open" and door["state"] == "closed":
        door["state"] = "open"
        return "ok" if env["role"] == "owner" else None
    if event == "remove":
        door["state"] = "gone"
        return {"removed": [1, 2.5, None, True], "calls": calls[0] > 0}
    return ("ok",)


def observe(door):
    return door["state"]
