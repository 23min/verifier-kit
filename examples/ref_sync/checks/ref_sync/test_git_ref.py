"""The workflow ref against real Git: scenarios C09-C11 and S6 of the chapter.

These check what the simulated remote in world.py assumes Git does, and that the client's
GitRemote publishes without touching code branches. Each check builds throwaway repositories
in a temporary folder, with git's global and system configuration switched off, and never
contacts any other host. They need the git program; without it they fail, never skip.
"""
import json
import os
import shutil
import subprocess
import sys
import tempfile
from contextlib import contextmanager
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import world as W                      # noqa: E402

REF = "refs/workflow/state"
GIT_ENV = {"GIT_CONFIG_GLOBAL": os.devnull, "GIT_CONFIG_NOSYSTEM": "1", "GIT_TERMINAL_PROMPT": "0",
           "GIT_AUTHOR_NAME": "ref-sync check", "GIT_AUTHOR_EMAIL": "check@example.invalid",
           "GIT_COMMITTER_NAME": "ref-sync check", "GIT_COMMITTER_EMAIL": "check@example.invalid"}


def git(cwd, *args, ok=True, stdin=None):
    p = subprocess.run(["git", *args], cwd=cwd, input=stdin, capture_output=True, text=True)
    if ok and p.returncode != 0:
        raise AssertionError(f"git {' '.join(args)} failed: {p.stderr.strip()}")
    return p


@contextmanager
def repos():
    """A remote with a code branch and a workflow ref, and a clone of it; yields (remote, clone)."""
    assert shutil.which("git"), "git not found: the ref-sync Git checks need the git program"
    saved = {k: os.environ.get(k) for k in GIT_ENV}
    os.environ.update(GIT_ENV)
    try:
        with tempfile.TemporaryDirectory() as tmp:
            remote, seed, clone = (str(Path(tmp, n)) for n in ("remote.git", "seed", "clone"))
            git(tmp, "init", "-q", "--bare", "-b", "main", remote)
            git(tmp, "init", "-q", "-b", "main", seed)
            Path(seed, "README").write_text("code\n")
            git(seed, "add", "README")
            git(seed, "commit", "-q", "-m", "code")
            git(seed, "push", "-q", remote, "main")
            root = commit(seed, W.FIRST_TREE, parent=None)
            git(seed, "push", "-q", remote, f"{root}:{REF}")
            git(tmp, "clone", "-q", remote, clone)
            yield remote, clone
    finally:
        for k, v in saved.items():
            if v is None:
                os.environ.pop(k, None)
            else:
                os.environ[k] = v


def commit(repo, files, parent):
    """A commit with exactly these files, built with plumbing, outside any branch or index."""
    index = str(Path(repo, ".git", "ref-sync-check-index"))
    env = dict(os.environ, GIT_INDEX_FILE=index)
    for path, text in sorted(files.items()):
        blob = git(repo, "hash-object", "-w", "--stdin", stdin=text).stdout.strip()
        subprocess.run(["git", "update-index", "--add", "--cacheinfo", f"100644,{blob},{path}"],
                       cwd=repo, env=env, check=True)
    tree = subprocess.run(["git", "write-tree"], cwd=repo, env=env, check=True,
                          capture_output=True, text=True).stdout.strip()
    os.remove(index)
    args = ["commit-tree", tree, "-m", "workflow event"] + (["-p", parent] if parent else [])
    return git(repo, *args).stdout.strip()


def tip(remote, ref=REF):
    return git(remote, "rev-parse", ref).stdout.strip()


def with_event(files, intent):
    out = dict(files)
    out[f"entities/{intent['entity']}.json"] = json.dumps({"value": intent["value"], "version": 1})
    out[f"events/{intent['event_id']}.json"] = json.dumps(intent)
    return out


ALICE_X = W.make_intent("e-alice", "alice", "x", "a")
BOB_Y = W.make_intent("e-bob", "bob", "y", "b")


def test_c09_git_refuses_a_push_that_is_not_a_fast_forward():
    with repos() as (remote, clone):
        git(clone, "fetch", "-q", remote, REF)
        root = git(clone, "rev-parse", "FETCH_HEAD").stdout.strip()
        first = commit(clone, with_event(W.FIRST_TREE, ALICE_X), root)
        git(clone, "push", "-q", remote, f"{first}:{REF}")
        stale = commit(clone, with_event(W.FIRST_TREE, BOB_Y), root)
        refused = git(clone, "push", "--porcelain", remote, f"{stale}:{REF}", ok=False)
        assert refused.returncode != 0, "git accepted a push that was not a fast-forward"
        assert tip(remote) == first, "a refused push moved the workflow ref"


def test_c09_git_remote_reports_non_fast_forward():
    import ref_sync_client
    with repos() as (remote, clone):
        gr = ref_sync_client.GitRemote(clone, remote)
        root, files = gr.fetch()
        assert gr.push(root, with_event(files, ALICE_X)) == "accepted"
        published = tip(remote)
        assert gr.push(root, with_event(files, BOB_Y)) == "non_fast_forward"
        assert tip(remote) == published, "a refused push moved the workflow ref"
        assert gr.fetch()[1] == with_event(W.FIRST_TREE, ALICE_X), "the fetched tree is not the published one"


def test_c10_s6_publishing_leaves_code_branches_alone():
    import ref_sync_client
    with repos() as (remote, clone):
        branches = git(remote, "for-each-ref", "refs/heads").stdout
        head = git(clone, "rev-parse", "HEAD").stdout.strip()
        local_refs = git(clone, "for-each-ref", "refs/heads", "refs/remotes").stdout
        gr = ref_sync_client.GitRemote(clone, remote)
        root, files = gr.fetch()
        assert gr.push(root, with_event(files, ALICE_X)) == "accepted"
        assert git(remote, "for-each-ref", "refs/heads").stdout == branches, "a code branch changed on the remote"
        assert git(clone, "rev-parse", "HEAD").stdout.strip() == head, "the clone's HEAD moved"
        assert git(clone, "for-each-ref", "refs/heads", "refs/remotes").stdout == local_refs, "a branch changed in the clone"
        assert git(clone, "status", "--porcelain").stdout == "", "the clone's index or working tree changed"
        shared = git(remote, "merge-base", "main", REF, ok=False)
        assert shared.returncode != 0, f"main and the workflow ref share commit {shared.stdout.strip()}"


def test_c11_a_fresh_clone_has_the_workflow_ref_only_after_fetching_it():
    import ref_sync_client
    with repos() as (remote, clone):
        gr = ref_sync_client.GitRemote(clone, remote)
        root, files = gr.fetch()
        assert gr.push(root, with_event(files, ALICE_X)) == "accepted"
        published = tip(remote)
        fresh = str(Path(clone).parent / "fresh")
        git(clone, "clone", "-q", remote, fresh)
        refs = git(fresh, "for-each-ref", "--format=%(objectname) %(refname)").stdout
        assert published not in refs, f"a plain clone already has the workflow ref: {refs.strip()}"
        got_tip, got_files = ref_sync_client.GitRemote(fresh, remote).fetch()
        assert got_tip == published and got_files == with_event(W.FIRST_TREE, ALICE_X)
