"""Notes service as the builder agent first wrote it: two bugs that conformance finds."""
from datetime import datetime, timezone


class PermissionDenied(Exception):
    pass


class InvalidState(Exception):
    pass


class Note:
    def __init__(self, note_id, owner):
        self.id = note_id
        self.owner = owner
        self.editors = set()
        self.text = ""
        self.published = False
        self.trashed_at = None


class NotesService:
    def __init__(self):
        self._notes = {}
        self._next_id = 1

    def create(self, user):
        note = Note(self._next_id, user)
        self._notes[note.id] = note
        self._next_id += 1
        return note.id

    def get(self, note_id):
        return self._notes.get(note_id)

    def share(self, note_id, user, editor):
        note = self._require(note_id)
        if user != note.owner:
            raise PermissionDenied("only the owner can share")
        note.editors.add(editor)

    def publish(self, note_id, user):
        note = self._require(note_id)
        if user != note.owner:
            raise PermissionDenied("only the owner can publish")
        if note.trashed_at:
            raise InvalidState("cannot publish a trashed note")
        note.published = True

    def edit(self, note_id, user, text):
        note = self._require(note_id)
        if user != note.owner and user not in note.editors:
            raise PermissionDenied("not allowed to edit")
        note.text = text

    def trash(self, note_id, user):
        note = self._require(note_id)
        if user != note.owner:
            raise PermissionDenied("only the owner can trash")
        if note.trashed_at is None:
            note.trashed_at = datetime.now(timezone.utc)

    def restore(self, note_id, user):
        note = self._require(note_id)
        if user != note.owner:
            raise PermissionDenied("only the owner can restore")
        note.trashed_at = None

    def delete_forever(self, note_id, user):
        note = self._require(note_id)
        if user != note.owner:
            raise PermissionDenied("only the owner can delete")
        if note.trashed_at is None:
            raise InvalidState("only notes in the trash can be deleted")
        del self._notes[note_id]

    def _require(self, note_id):
        note = self._notes.get(note_id)
        if note is None:
            raise InvalidState("no such note")
        return note
