"""Journaled workflow events published through a separate, fast-forward Git ref."""

import copy
import json
import os
import subprocess
import tempfile


FINAL = {"confirmed", "conflicted", "protocol_error"}
TRANSITIONS = {
    "queued": {
        "prepare": "prepared", "seen_on_remote": "confirmed",
        "absent_on_remote": "queued", "digest_mismatch": "protocol_error",
        "invalidated": "conflicted", "restart": "queued",
    },
    "prepared": {
        "accepted": "confirmed", "non_fast_forward": "queued",
        "response_lost": "uncertain", "restart": "queued",
    },
    "uncertain": {
        "seen_on_remote": "confirmed", "absent_on_remote": "queued",
        "digest_mismatch": "protocol_error", "restart": "queued",
    },
}


def transition(state, event):
    """Return the permitted next state, or None for a refused event."""
    return TRANSITIONS.get(state, {}).get(event)


class Client:
    def __init__(self, name, remote, journal):
        self.name = name
        self.remote = remote
        self.journal = journal
        self.candidate = None
        for record in journal.values():
            if record["state"] not in FINAL:
                record["state"] = transition(record["state"], "restart")

    def submit(self, intent):
        self.journal[intent["event_id"]] = {
            "intent": copy.deepcopy(intent), "state": "queued",
        }

    def step(self):
        record = next((r for r in self.journal.values() if r["state"] not in FINAL), None)
        if record is None:
            return False
        state = record["state"]
        if state == "prepared":
            parent, files = self.candidate
            try:
                event = self.remote.push(parent, files, force=False)
            except TimeoutError:
                event = "response_lost"
            record["state"] = transition(state, event)
            self.candidate = None
            return True

        try:
            tip, files = self.remote.fetch()
        except OSError:
            return True
        intent = record["intent"]
        event_path = f"events/{intent['event_id']}.json"
        if event_path in files:
            published = json.loads(files[event_path])
            event = ("seen_on_remote" if published["digest"] == intent["digest"]
                     else "digest_mismatch")
        elif state == "uncertain":
            event = "absent_on_remote"
        else:
            entity_path = f"entities/{intent['entity']}.json"
            entity = json.loads(files[entity_path])
            if entity["version"] != intent["expected_version"]:
                event = "invalidated"
            else:
                candidate = dict(files)
                candidate[entity_path] = json.dumps({
                    "value": intent["value"], "version": entity["version"] + 1,
                })
                candidate[event_path] = json.dumps(intent)
                self.candidate = (tip, candidate)
                event = "prepare"
        record["state"] = transition(state, event)
        return True


class GitRemote:
    REF = "refs/workflow/state"

    def __init__(self, repo, url):
        self.repo = os.fspath(repo)
        self.url = os.fspath(url)

    def _git(self, *args, stdin=None, env=None, check=True):
        result = subprocess.run(
            ["git", *args], cwd=self.repo, input=stdin, env=env,
            capture_output=True, text=True,
        )
        if check and result.returncode:
            raise OSError(result.stderr.strip())
        return result

    def fetch(self):
        self._git("fetch", "--no-tags", "--no-recurse-submodules",
                  "--refmap=", self.url, self.REF)
        tip = self._git("rev-parse", "FETCH_HEAD").stdout.strip()
        files = {}
        entries = self._git("ls-tree", "-r", "-z", tip).stdout
        for entry in entries.split("\0"):
            if entry:
                metadata, path = entry.split("\t", 1)
                _, kind, oid = metadata.split()
                if kind != "blob":
                    raise ValueError("workflow tree must contain files only")
                files[path] = self._git("cat-file", "blob", oid).stdout
        return tip, files

    def push(self, parent, files, force=False):
        if force:
            raise ValueError("workflow publications must not be forced")
        # A separate index builds the complete tree without touching the checkout.
        with tempfile.TemporaryDirectory(prefix="ref-sync-") as tmp:
            env = dict(os.environ, GIT_INDEX_FILE=os.path.join(tmp, "index"))
            self._git("read-tree", "--empty", env=env)
            for path, content in sorted(files.items()):
                blob = self._git("hash-object", "-w", "--stdin", stdin=content).stdout.strip()
                self._git("update-index", "--add", "--cacheinfo",
                          "100644", blob, path, env=env)
            tree = self._git("write-tree", env=env).stdout.strip()
        # Fixed metadata makes a retry of the same candidate deterministic.
        env = dict(os.environ, GIT_AUTHOR_NAME="ref-sync",
                   GIT_AUTHOR_EMAIL="ref-sync@example.invalid",
                   GIT_COMMITTER_NAME="ref-sync",
                   GIT_COMMITTER_EMAIL="ref-sync@example.invalid",
                   GIT_AUTHOR_DATE="2000-01-01T00:00:00+0000",
                   GIT_COMMITTER_DATE="2000-01-01T00:00:00+0000")
        commit = self._git("commit-tree", tree, "-p", parent,
                           "-m", "Publish workflow event", env=env).stdout.strip()
        result = self._git("push", "--porcelain", self.url,
                           f"{commit}:{self.REF}", check=False)
        if result.returncode == 0:
            return "accepted"
        if any(reason in result.stdout for reason in ("(non-fast-forward)", "(fetch first)")):
            return "non_fast_forward"
        # Other failures cannot establish whether the remote updated its ref.
        raise TimeoutError(result.stderr.strip() or result.stdout.strip())
