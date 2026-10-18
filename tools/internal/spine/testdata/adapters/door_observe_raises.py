"""observe() raises once the door is gone: adapter-failed after the sequence that got there."""

def new():
    return {"state": "closed"}


def apply(door, event, env):
    if event == "remove" and door["state"] == "closed":
        door["state"] = "gone"
    elif event == "open" and door["state"] == "closed" and env["role"] == "owner":
        door["state"] = "open"
    elif event == "open" and door["state"] == "closed":
        return "rejected"
    return "ok"


def observe(door):
    if door["state"] == "gone":
        raise KeyError("gone")
    return door["state"]
