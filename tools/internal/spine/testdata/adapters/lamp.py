"""An adapter for diff/plain-a.yaml, a lamp with no variables, whose second press leaves it lit."""

def new():
    return {"state": "dark"}


def apply(lamp, event, env):
    if lamp["state"] == "broken":
        return "rejected" if event == "press" else "ok"
    if event == "drop":
        lamp["state"] = "broken"
    elif lamp["state"] == "dark":
        lamp["state"] = "lit"
    return "ok"


def observe(lamp):
    return lamp["state"]
