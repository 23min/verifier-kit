"""A correct adapter for diff/stop-a.yaml: go moves s to t, and is refused in t."""

def new():
    return {"state": "s"}


def apply(m, event, env):
    if m["state"] == "t":
        return "rejected"
    m["state"] = "t"
    return "ok"


def observe(m):
    return m["state"]
