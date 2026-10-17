"""apply() raises for guests: each is a divergence, shown as crashed (...)."""

def new():
    return {"state": "closed"}


def apply(door, event, env):
    if env["role"] == "guest":
        raise ValueError(f"guests can't {event}")
    if event == "open" and door["state"] == "closed":
        door["state"] = "open"
    elif event == "remove" and door["state"] == "closed":
        door["state"] = "gone"
    return "ok"


def observe(door):
    return door["state"]
