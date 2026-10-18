"""observe() returns a set once the door opens, which JSON can't carry: the shim fails."""

def new():
    return {"state": "closed"}


def apply(door, event, env):
    if event == "open" and door["state"] == "closed":
        if env["role"] != "owner":
            return "rejected"
        door["state"] = "open"
    elif event == "remove" and door["state"] == "closed":
        door["state"] = "gone"
    return "ok"


def observe(door):
    return {door["state"]} if door["state"] == "open" else door["state"]
