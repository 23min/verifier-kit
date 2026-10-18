"""A correct adapter for seeds/spine.yaml: a door that owners open and anyone removes."""

class Door:
    state = "closed"


def new():
    return Door()


def apply(door, event, env):
    if door.state == "closed" and event == "open":
        if env["role"] != "owner":
            return "rejected"
        door.state = "open"
    elif door.state == "closed" and event == "remove":
        door.state = "gone"
    return "ok"


def observe(door):
    return door.state
