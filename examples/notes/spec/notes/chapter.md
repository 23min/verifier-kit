# notes: notes, sharing and the trash

A notes service keeps notes that people write and share with each other. This chapter says how
one note moves between being a draft, published, in the trash and deleted for good.

## 1. Glossary

- **draft**: a note that has not been published
- **active**: a published note
- **trashed**: a note in the trash
- **purged**: a note deleted for good (cannot come back)
- events: publish, edit, trash, restore, purge
- roles: owner, editor, viewer

## 2. Trash

Owners can move a note to the trash. A trashed note can be restored.
Notes in the trash are deleted for good when the owner empties the trash.
Viewers can only read notes. Editors can change a note's content.

## 3. What must hold

- A note's text is the text it was last given: an edit stores it as given, and trashing and
  restoring the note keep it.

## 4. Scenarios

| Scenario | What must happen |
|---|---|
| The owner publishes a note, and an editor edits it | The note stays active |
| The owner publishes a note, trashes it and restores it | The note is active again |

## 5. Assumptions

None.

## 6. Not covered

None.

## 7. What the lifecycle does not show

The lifecycle does not show a note's text. Whether edits, trash and restore keep it is checked
by the content checks.
