"""spines/orders.yaml's order, as code: each state takes the one event that moves it on. BROKEN, set
by orders_bug.py and orders_stuck.py, is the one move this code refuses although the spine allows it."""

STATES = ["created", "paid", "packed", "shipped", "delivered", "returned"]
EVENTS = ["pay", "pack", "ship", "deliver", "return_item"]
BROKEN = None


def new():
    return {"at": 0}


def apply(order, event, env):
    if EVENTS.index(event) != order["at"] or event == BROKEN:
        return "rejected"
    order["at"] += 1
    return "ok"


def observe(order):
    return STATES[order["at"]]
