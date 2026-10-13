"""Connects the spine to the notes service.

Owned by the oracle side: the builder can read it but not change it.
It only translates. It never decides whether an action is allowed.

  new()      -> a fresh service with one note, shared with an editor
  apply()    -> calls the service for one spine event; returns "ok" or "rejected"
  observe()  -> reads the service's data and names the spine state it is in
"""
import importlib
import os

USERS = {"owner": "olivia", "editor": "eddie", "viewer": "vera"}


def new():
    app = importlib.import_module(os.environ.get("NOTES_APP", "notes_app"))
    svc = app.NotesService()
    note = svc.create("olivia")
    svc.share(note, "olivia", "eddie")
    return {"app": app, "svc": svc, "note": note}


def apply(h, event, params):
    svc, note, user = h["svc"], h["note"], USERS[params["role"]]
    calls = {
        "publish": lambda: svc.publish(note, user),
        "edit": lambda: svc.edit(note, user, "new text"),
        "trash": lambda: svc.trash(note, user),
        "restore": lambda: svc.restore(note, user),
        "purge": lambda: svc.delete_forever(note, user),
    }
    try:
        calls[event]()
        return "ok"
    except (h["app"].PermissionDenied, h["app"].InvalidState):
        return "rejected"


def observe(h):
    n = h["svc"].get(h["note"])
    if n is None:
        return "purged"
    if n.trashed_at is not None:
        return "trashed"
    return "active" if n.published else "draft"
