"""chain9.py, but a second edit in a row is refused: margin 1 tries two edits in a row, and finds
it; margin 0 tries each case once, and doesn't."""
import chain9
from chain9 import observe


def new():
    chain = chain9.new()
    chain["last"] = None
    return chain


def apply(chain, event, env):
    twice = event == "edit" and chain["last"] == "edit"
    chain["last"] = event
    return "rejected" if twice else chain9.apply(chain, event, env)
