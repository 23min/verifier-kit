"""A correct adapter for drawings/outputs/spine.yaml (#116): the door of seeds/spine.yaml, which
chimes when its owner opens it, or opens it again, and sounds the alarm when it is removed."""


class Door:
    def __init__(self):
        self.state, self.sent = "closed", []


def new():
    return Door()


def apply(door, event, env):
    door.sent = []
    if door.state == "closed" and event == "open":
        if env["role"] != "owner":
            return "rejected"
        door.state = "open"
        door.sent.append("chime")
    elif door.state == "open" and event == "open" and env["role"] == "owner":
        door.sent.append("chime")
    elif door.state == "closed" and event == "remove":
        door.state = "gone"
        door.sent.append("alarm")
    return "ok"


def observe(door):
    return door.state


def outputs(door):
    return door.sent
