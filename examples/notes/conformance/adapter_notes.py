"""Connects the spine to the notes service.

Owned by the oracle side: the builder can read it but not change it.
It only translates. It never decides whether an action is allowed.

  new()      -> a fresh service with one note, shared with an editor
  apply()    -> calls the service for one spine event; returns "ok" or "rejected"
  observe()  -> reads the service's data and names the spine state it is in

What src/ must offer, in a module named notes_app. The checks in checks/notes/ use the same
names, so this is the whole contract between the oracle and the service.

  NotesService() -> a service holding no notes
  svc.create(user) -> the ID of a new draft note, owned by user
  svc.share(note, owner, editor)
            lets editor edit the note.
  svc.publish(note, user), svc.edit(note, user, text), svc.trash(note, user),
  svc.restore(note, user), svc.delete_forever(note, user)
            each does what it names, or raises PermissionDenied when the user may not, or
            InvalidState when the note's state doesn't allow it. Both are the module's own
            exception classes.
  svc.get(note) -> the note, or None once it is deleted for good. A note has .text, .published
            (True once published) and .trashed_at (None unless it is in the trash).
"""

USERS = {"owner": "olivia", "editor": "eddie", "viewer": "vera"}


def new():
    import notes_app  # here, so that a service that can't be imported fails new(), with the reason

    svc = notes_app.NotesService()
    note = svc.create("olivia")
    svc.share(note, "olivia", "eddie")
    return {"app": notes_app, "svc": svc, "note": note}


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
