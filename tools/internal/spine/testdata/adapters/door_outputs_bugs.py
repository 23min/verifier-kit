"""door_outputs.py with a bug in each kind of send (#116): a guest refused at the closed door
chimes anyway, removing the door sounds no alarm, the owner's opening of an open door chimes
twice, and a guest at the open door chimes. Its states and answers are all right."""


class Door:
    def __init__(self):
        self.state, self.sent = "closed", []


def new():
    return Door()


def apply(door, event, env):
    door.sent = []
    if door.state == "closed" and event == "open":
        door.sent.append("chime")
        if env["role"] != "owner":
            return "rejected"
        door.state = "open"
    elif door.state == "open" and event == "open":
        door.sent += ["chime", "chime"] if env["role"] == "owner" else ["chime"]
    elif door.state == "closed" and event == "remove":
        door.state = "gone"
    return "ok"


def observe(door):
    return door.state


def outputs(door):
    return door.sent
