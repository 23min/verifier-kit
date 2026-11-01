"""spines/chain9.yaml's chain, as code."""


def new():
    return {"at": 1}


def apply(chain, event, env):
    role = env["role"]
    if event == "next" and role == "owner":
        chain["at"] += 1
        return "ok"
    if event == "edit" and role != "viewer" or event == "touch" and role == "owner":
        return "ok"
    return "rejected"


def observe(chain):
    return "done" if chain["at"] == 10 else "s%d" % chain["at"]
