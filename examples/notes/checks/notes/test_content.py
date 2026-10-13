"""Content checks for notes: what the spine cannot see.

The spine describes states and permissions, not a note's text. These checks cover the
text, and each one cites the spine cell it belongs to, so `spine stale` can tell when
the cell changes and the check needs another look.
"""
from notes_app import NotesService


def _note(svc, published):
    note = svc.create("olivia")
    svc.share(note, "olivia", "eddie")
    if published:
        svc.publish(note, "olivia")
    return note


# covers: draft/edit@4a79e65e9c2b
def test_editor_edit_of_draft_keeps_text():
    svc = NotesService()
    note = _note(svc, published=False)
    svc.edit(note, "eddie", "Draft by Eddie")
    assert svc.get(note).text == "Draft by Eddie"


# covers: active/edit@a6899d5a7722
def test_owner_edit_of_published_note_keeps_text():
    svc = NotesService()
    note = _note(svc, published=True)
    svc.edit(note, "olivia", "Shopping: milk, eggs")
    assert svc.get(note).text == "Shopping: milk, eggs"


# covers: trashed/restore@95f9c8204e9f
def test_restore_keeps_text():
    svc = NotesService()
    note = _note(svc, published=True)
    svc.edit(note, "olivia", "Keep me")
    svc.trash(note, "olivia")
    svc.restore(note, "olivia")
    assert svc.get(note).text == "Keep me"
