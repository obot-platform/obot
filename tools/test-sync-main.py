#!/usr/bin/env python3
"""Exercise the sync against real, isolated Git repositories; no network calls."""

import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import textwrap
import unittest


SCRIPT = Path(__file__).resolve().with_name("sync-main.sh")
GIT = shutil.which("git")

# Records every Git call the script makes and can inject a real ref update into
# the destination at a chosen point. Git itself still performs each operation
# and enforces the explicit lease.
GIT_SHIM = textwrap.dedent(
    """\
    #!/usr/bin/env python3
    import json, os, subprocess, sys

    args = sys.argv[1:]
    log_path = os.environ["GIT_CALL_LOG"]
    with open(log_path, "a") as log:
        log.write(json.dumps(args) + "\\n")
    with open(log_path) as log:
        calls = [json.loads(line) for line in log]
    destination = os.environ["RACE_REPO"]
    when = os.environ.get("RACE_WHEN")

    def race():
        subprocess.run(
            [os.environ["REAL_GIT"], "-C", destination,
             "update-ref", "refs/heads/main", os.environ["RACE_SHA"]],
            check=True,
        )

    def reads_destination(call):
        return "ls-remote" in call and destination in call

    is_push = "push" in args
    # The second read of the destination is the final verification.
    is_verify = reads_destination(args) and sum(map(reads_destination, calls)) == 2
    if (when == "before-push" and is_push) or (when == "before-verify" and is_verify):
        race()
    result = subprocess.run([os.environ["REAL_GIT"], *args])
    if when == "after-push" and is_push and result.returncode == 0:
        race()
    sys.exit(result.returncode)
    """
)


class SyncMainTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="test-sync-main-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.env = {
            "PATH": os.environ["PATH"],
            "HOME": str(self.root),
            "TMPDIR": str(self.root),
            "LANG": "C.UTF-8",
            "GIT_CONFIG_NOSYSTEM": "1",
            "GIT_CONFIG_GLOBAL": os.devnull,
            "GIT_AUTHOR_NAME": "Sync Test",
            "GIT_AUTHOR_EMAIL": "sync-test@example.invalid",
            "GIT_COMMITTER_NAME": "Sync Test",
            "GIT_COMMITTER_EMAIL": "sync-test@example.invalid",
        }
        self.objects = self.make_repo("objects")
        self.source = self.make_repo("source")
        self.destination = self.make_repo("destination")
        self.base = self.commit("base")
        self.publish(self.source, self.base)
        self.publish(self.destination, self.base)
        self.private = self.commit("private work", self.base)
        self.publish(self.destination, self.private, "refs/heads/security-work")
        self.publish(self.destination, self.private, "refs/tags/private-tag")
        self.git_calls = []

    def git(self, repo, *args, input=None, check=True):
        return subprocess.run(
            [GIT, "-C", str(repo), *map(str, args)],
            input=input,
            text=True,
            capture_output=True,
            env=self.env,
            check=check,
        )

    def make_repo(self, name, *init_args):
        path = self.root / name
        subprocess.run(
            [GIT, "init", "--bare", "--quiet", "--initial-branch=main", *init_args, str(path)],
            check=True,
            env=self.env,
        )
        return path

    def commit(self, message, parent=None, repo=None):
        repo = repo or self.objects
        blob = self.git(repo, "hash-object", "-w", "--stdin", input=message).stdout.strip()
        tree = self.git(repo, "mktree", input=f"100644 blob {blob}\tcontent.txt\n").stdout.strip()
        args = ["commit-tree", tree, "-m", message]
        if parent:
            args += ["-p", parent]
        return self.git(repo, *args).stdout.strip()

    def publish(self, repo, sha, ref="refs/heads/main"):
        self.git(self.objects, "push", "--quiet", repo, f"+{sha}:{ref}")

    def refs(self, repo):
        output = self.git(repo, "for-each-ref", "--format=%(refname) %(objectname)").stdout
        return dict(line.split() for line in output.splitlines())

    def main(self, repo):
        return self.git(repo, "rev-parse", "refs/heads/main").stdout.strip()

    def ran(self, command):
        return any(command in call for call in self.git_calls)

    def run_sync(self, *, race=None, success=True):
        shim_dir = self.root / "shim"
        shim_dir.mkdir(exist_ok=True)
        shim = shim_dir / "git"
        shim.write_text(GIT_SHIM)
        shim.chmod(0o755)
        call_log = self.root / "git-calls.jsonl"
        call_log.write_text("")
        env = self.env | {
            "PATH": f"{shim_dir}:{self.env['PATH']}",
            "REAL_GIT": GIT,
            "GIT_CALL_LOG": str(call_log),
            "RACE_REPO": str(self.destination),
            "RACE_SHA": self.private,
        }
        if race:
            env["RACE_WHEN"] = race
        source_before = self.refs(self.source)
        other_refs_before = self.refs(self.destination)
        other_refs_before.pop("refs/heads/main", None)
        result = subprocess.run(
            ["bash", str(SCRIPT), str(self.source), str(self.destination)],
            text=True,
            capture_output=True,
            env=env,
            # Run outside any repository, as a fresh CI runner would.
            cwd=self.root,
        )
        self.git_calls = [json.loads(line) for line in call_log.read_text().splitlines()]
        if success:
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        else:
            self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.refs(self.source), source_before, "Source refs changed")
        other_refs_after = self.refs(self.destination)
        other_refs_after.pop("refs/heads/main", None)
        self.assertEqual(other_refs_after, other_refs_before, "Non-main destination refs changed")
        # Successful and failed runs must clean up their temporary bare repo.
        self.assertFalse(list(self.root.glob("tmp.*")))
        return result

    def reject_pushes(self):
        hook = self.destination / "hooks" / "pre-receive"
        hook.write_text("#!/bin/sh\necho 'simulated branch protection' >&2\nexit 1\n")
        hook.chmod(0o755)

    def test_fast_forward_preserves_exact_history_and_unrelated_refs(self):
        next_sha = self.commit("upstream update", self.base)
        self.publish(self.source, next_sha)
        self.publish(self.source, next_sha, "refs/heads/another-upstream-branch")
        self.publish(self.source, next_sha, "refs/tags/upstream-tag")
        result = self.run_sync()
        self.assertEqual(self.main(self.destination), next_sha)
        self.assertEqual(
            self.git(self.destination, "rev-list", "main").stdout,
            self.git(self.source, "rev-list", "main").stdout,
        )
        self.assertEqual(
            self.git(self.destination, "ls-tree", "-r", "main").stdout,
            self.git(self.source, "ls-tree", "-r", "main").stdout,
        )
        self.assertIn(
            f"Fast-forwarding destination main from {self.base} to {next_sha}.", result.stdout
        )
        self.assertNotIn("Warning", result.stderr)

    def test_replaces_divergent_main_but_keeps_private_branch(self):
        self.publish(self.destination, self.private)
        public = self.commit("public update", self.base)
        self.publish(self.source, public)
        result = self.run_sync()
        self.assertEqual(self.main(self.destination), public)
        self.assertEqual(self.refs(self.destination)["refs/heads/security-work"], self.private)
        self.assertIn(
            f"Warning: Destination main {self.private} is not in upstream history.", result.stderr
        )

    def test_source_history_can_be_rewritten(self):
        rewritten = self.commit("new unrelated upstream root")
        self.publish(self.source, rewritten)
        result = self.run_sync()
        self.assertEqual(self.main(self.destination), rewritten)
        self.assertEqual(self.git(self.destination, "rev-list", "--count", "main").stdout.strip(), "1")
        self.assertIn(f"Destination main {self.base} is not in upstream history.", result.stderr)

    def test_repeated_sync_is_a_no_op_even_when_pushes_are_rejected(self):
        self.publish(self.source, self.commit("public update", self.base))
        self.run_sync()
        self.reject_pushes()
        self.run_sync()
        self.assertEqual(self.main(self.destination), self.main(self.source))
        self.assertFalse(self.ran("fetch"), "A no-op run fetched upstream history")
        self.assertFalse(self.ran("push"), "A no-op run pushed")

    def test_concurrent_update_rejects_the_stale_lease(self):
        self.publish(self.source, self.commit("public update", self.base))
        result = self.run_sync(race="before-push", success=False)
        self.assertIn("stale info", result.stderr)
        self.assertEqual(self.main(self.destination), self.private)

    def test_post_push_verification_detects_a_concurrent_update(self):
        self.publish(self.source, self.commit("public update", self.base))
        result = self.run_sync(race="after-push", success=False)
        self.assertIn("changed during verification", result.stderr)
        self.assertEqual(self.main(self.destination), self.private)

    def test_no_op_verification_detects_a_concurrent_update(self):
        result = self.run_sync(race="before-verify", success=False)
        self.assertIn("changed during verification", result.stderr)
        self.assertEqual(self.main(self.destination), self.private)
        self.assertFalse(self.ran("push"))

    def test_rejected_push_leaves_destination_unchanged(self):
        self.publish(self.source, self.commit("public update", self.base))
        self.reject_pushes()
        self.run_sync(success=False)
        self.assertEqual(self.main(self.destination), self.base)

    def test_ignores_branches_whose_names_end_in_main(self):
        # ls-remote's refs/heads/main pattern also matches these by suffix.
        self.publish(self.destination, self.private, "refs/heads/backup/refs/heads/main")
        self.publish(self.source, self.private, "refs/heads/mirror/refs/heads/main")
        public = self.commit("public update", self.base)
        self.publish(self.source, public)
        self.run_sync()
        self.assertEqual(self.main(self.destination), public)

    def test_missing_source_main_leaves_destination_unchanged(self):
        self.git(self.source, "update-ref", "-d", "refs/heads/main")
        result = self.run_sync(success=False)
        self.assertIn("The source repository", result.stderr)
        self.assertIn("has no main branch", result.stderr)
        self.assertEqual(self.main(self.destination), self.base)

    def test_missing_destination_main_is_not_recreated(self):
        self.git(self.destination, "update-ref", "-d", "refs/heads/main")
        result = self.run_sync(success=False)
        self.assertIn("The destination repository", result.stderr)
        self.assertIn("has no main branch", result.stderr)
        self.assertNotIn("refs/heads/main", self.refs(self.destination))

    def test_rejects_main_that_is_not_a_sha1_commit(self):
        self.destination = self.make_repo("sha256-destination", "--object-format=sha256")
        sha256 = self.commit("sha256 main", repo=self.destination)
        self.git(self.destination, "update-ref", "refs/heads/main", sha256)
        result = self.run_sync(success=False)
        self.assertIn("SHA-1 commit ID", result.stderr)
        self.assertEqual(self.main(self.destination), sha256)
        self.assertFalse(self.ran("push"))


if __name__ == "__main__":
    unittest.main(verbosity=2)
