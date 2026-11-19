#!/usr/bin/env python3
"""Exercise the sync against real, isolated Git repositories; no network calls."""

import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


SCRIPT = Path(__file__).resolve().with_name("sync-main.sh")
GIT = shutil.which("git")


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

    def git(self, repo, *args, input=None, check=True):
        return subprocess.run(
            [GIT, "-C", str(repo), *map(str, args)],
            input=input,
            text=True,
            capture_output=True,
            env=self.env,
            check=check,
        )

    def make_repo(self, name):
        path = self.root / name
        subprocess.run(
            [GIT, "init", "--bare", "--quiet", "--initial-branch=main", str(path)],
            check=True,
            env=self.env,
        )
        return path

    def commit(self, message, parent=None):
        blob = self.git(self.objects, "hash-object", "-w", "--stdin", input=message).stdout.strip()
        tree = self.git(
            self.objects, "mktree", input=f"100644 blob {blob}\tcontent.txt\n"
        ).stdout.strip()
        args = ["commit-tree", tree, "-m", message]
        if parent:
            args += ["-p", parent]
        return self.git(self.objects, *args).stdout.strip()

    def publish(self, repo, sha, ref="refs/heads/main"):
        self.git(self.objects, "push", "--quiet", repo, f"+{sha}:{ref}")

    def refs(self, repo):
        output = self.git(repo, "for-each-ref", "--format=%(refname) %(objectname)").stdout
        return dict(line.split() for line in output.splitlines())

    def main(self, repo):
        return self.git(repo, "rev-parse", "refs/heads/main").stdout.strip()

    def run_sync(self, *, race=None, success=True):
        env = self.env.copy()
        if race:
            # Inject a real ref update immediately before or after Git's push.
            # Git itself still performs the push and enforces the explicit lease.
            shim_dir = self.root / "shim"
            shim_dir.mkdir()
            shim = shim_dir / "git"
            shim.write_text(
                "#!/usr/bin/env python3\n"
                "import os, subprocess, sys\n"
                "args = sys.argv[1:]\n"
                "is_push = 'push' in args\n"
                "def race():\n"
                "    subprocess.run([os.environ['REAL_GIT'], '-C', os.environ['RACE_REPO'],\n"
                "                    'update-ref', 'refs/heads/main', os.environ['RACE_SHA']], check=True)\n"
                "if is_push and os.environ['RACE_WHEN'] == 'before': race()\n"
                "result = subprocess.run([os.environ['REAL_GIT'], *args])\n"
                "if is_push and result.returncode == 0 and os.environ['RACE_WHEN'] == 'after': race()\n"
                "sys.exit(result.returncode)\n"
            )
            shim.chmod(0o755)
            env.update(
                PATH=f"{shim_dir}:{env['PATH']}",
                REAL_GIT=GIT,
                RACE_REPO=str(self.destination),
                RACE_SHA=self.private,
                RACE_WHEN=race,
            )
        source_before = self.refs(self.source)
        other_refs_before = self.refs(self.destination)
        other_refs_before.pop("refs/heads/main", None)
        result = subprocess.run(
            ["bash", str(SCRIPT), str(self.source), str(self.destination)],
            text=True,
            capture_output=True,
            env=env,
        )
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
        self.run_sync()
        self.assertEqual(self.main(self.destination), next_sha)
        self.assertEqual(
            self.git(self.destination, "rev-list", "main").stdout,
            self.git(self.source, "rev-list", "main").stdout,
        )
        self.assertEqual(
            self.git(self.destination, "ls-tree", "-r", "main").stdout,
            self.git(self.source, "ls-tree", "-r", "main").stdout,
        )

    def test_replaces_divergent_main_but_keeps_private_branch(self):
        self.publish(self.destination, self.private)
        public = self.commit("public update", self.base)
        self.publish(self.source, public)
        self.run_sync()
        self.assertEqual(self.main(self.destination), public)
        self.assertEqual(self.refs(self.destination)["refs/heads/security-work"], self.private)

    def test_source_history_can_be_rewritten(self):
        rewritten = self.commit("new unrelated upstream root")
        self.publish(self.source, rewritten)
        self.run_sync()
        self.assertEqual(self.main(self.destination), rewritten)
        self.assertEqual(self.git(self.destination, "rev-list", "--count", "main").stdout.strip(), "1")

    def test_repeated_sync_is_a_no_op_even_when_pushes_are_rejected(self):
        self.publish(self.source, self.commit("public update", self.base))
        self.run_sync()
        self.reject_pushes()
        self.run_sync()
        self.assertEqual(self.main(self.destination), self.main(self.source))

    def test_concurrent_update_rejects_the_stale_lease(self):
        self.publish(self.source, self.commit("public update", self.base))
        result = self.run_sync(race="before", success=False)
        self.assertIn("stale info", result.stderr)
        self.assertEqual(self.main(self.destination), self.private)

    def test_post_push_verification_detects_a_concurrent_update(self):
        self.publish(self.source, self.commit("public update", self.base))
        result = self.run_sync(race="after", success=False)
        self.assertIn("changed during verification", result.stderr)
        self.assertEqual(self.main(self.destination), self.private)

    def test_rejected_push_leaves_destination_unchanged(self):
        self.publish(self.source, self.commit("public update", self.base))
        self.reject_pushes()
        self.run_sync(success=False)
        self.assertEqual(self.main(self.destination), self.base)

    def test_missing_source_main_leaves_destination_unchanged(self):
        self.git(self.source, "update-ref", "-d", "refs/heads/main")
        self.run_sync(success=False)
        self.assertEqual(self.main(self.destination), self.base)

    def test_missing_destination_main_is_not_recreated(self):
        self.git(self.destination, "update-ref", "-d", "refs/heads/main")
        self.run_sync(success=False)
        self.assertNotIn("refs/heads/main", self.refs(self.destination))


if __name__ == "__main__":
    unittest.main(verbosity=2)
