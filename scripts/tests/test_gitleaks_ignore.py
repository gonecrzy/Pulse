#!/usr/bin/env python3
"""Keep historical gitleaks suppressions exact and auditable."""

from __future__ import annotations

import importlib.util
import re
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch


REPO_ROOT = Path(__file__).resolve().parents[2]
IGNORE_PATH = REPO_ROOT / ".gitleaksignore"
HISTORICAL_FINGERPRINT = re.compile(
    r"^(?P<commit>[0-9a-f]{40}):(?P<path>.+):(?P<rule>[a-z0-9-]+):(?P<line>[1-9][0-9]*)$"
)
CURRENT_FINGERPRINT = re.compile(
    r"^(?P<path>.+):(?P<rule>[a-z0-9-]+):(?P<line>[1-9][0-9]*)$"
)
HISTORY_SCRIPT = REPO_ROOT / "scripts" / "fetch_gitleaks_history.py"
HISTORY_SPEC = importlib.util.spec_from_file_location("fetch_gitleaks_history", HISTORY_SCRIPT)
history = importlib.util.module_from_spec(HISTORY_SPEC)
assert HISTORY_SPEC.loader is not None
HISTORY_SPEC.loader.exec_module(history)


def ignore_entries() -> list[str]:
    return [
        line.strip()
        for line in IGNORE_PATH.read_text(encoding="utf-8").splitlines()
        if line.strip() and not line.lstrip().startswith("#")
    ]


class GitleaksIgnoreTest(unittest.TestCase):
    def test_entries_are_exact_fingerprints(self) -> None:
        invalid = [
            entry
            for entry in ignore_entries()
            if not HISTORICAL_FINGERPRINT.fullmatch(entry)
            and not CURRENT_FINGERPRINT.fullmatch(entry)
        ]
        self.assertEqual(invalid, [], msg=f"non-exact gitleaks ignore entries: {invalid}")

    def test_historical_fingerprints_resolve_to_the_named_blob(self) -> None:
        failures: list[str] = []

        for entry in ignore_entries():
            match = HISTORICAL_FINGERPRINT.fullmatch(entry)
            if match is None:
                continue

            commit = match.group("commit")
            path = match.group("path")
            result = subprocess.run(
                ["git", "cat-file", "-e", f"{commit}:{path}"],
                cwd=REPO_ROOT,
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
                check=False,
            )
            if result.returncode != 0:
                failures.append(entry)

        self.assertEqual(
            failures,
            [],
            msg="historical gitleaks fingerprints do not resolve:\n- "
            + "\n- ".join(failures),
        )

    def test_missing_history_fetches_only_exact_fingerprint_objects(self) -> None:
        entries = ignore_entries()
        expected = sorted({
            match.group("commit")
            for entry in entries
            if (match := HISTORICAL_FINGERPRINT.fullmatch(entry))
        })
        calls: list[list[str]] = []
        fetched = False

        def fake_run(command: list[str], **kwargs: object) -> subprocess.CompletedProcess:
            nonlocal fetched
            calls.append(command)
            if command[1:3] == ["init", "--bare"]:
                return subprocess.CompletedProcess(command, 0)
            if "fetch" in command:
                fetched = True
                return subprocess.CompletedProcess(command, 0)
            if "cat-file" in command:
                return subprocess.CompletedProcess(command, 0 if fetched else 1)
            self.fail(f"unexpected Git command: {command}")

        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            env_file = root / "github-env"
            with patch.object(history.subprocess, "run", side_effect=fake_run):
                count = history.fetch_missing_objects(
                    root=root,
                    ignore_path=IGNORE_PATH,
                    runner_temp=root,
                    github_env=env_file,
                )
            env_contents = env_file.read_text()

        fetches = [command for command in calls if "fetch" in command]
        self.assertEqual(count, len(expected))
        self.assertEqual(len(fetches), 1)
        self.assertEqual(fetches[0], ["git", "--git-dir", str(root / "gitleaks-history.git"),
                                      "fetch", "--no-tags", "--depth=1", history.UPSTREAM_URL, *expected])
        self.assertTrue(all("checkout" not in command for command in calls))
        self.assertEqual(env_contents, f"GIT_ALTERNATE_OBJECT_DIRECTORIES={root / 'gitleaks-history.git' / 'objects'}\n")

    def test_build_and_test_loads_history_before_script_smoke_without_checkout(self) -> None:
        workflow = (REPO_ROOT / ".github" / "workflows" / "build-and-test.yml").read_text(encoding="utf-8")
        step = "      - name: Fetch missing historical gitleaks fingerprint objects\n        run: python3 scripts/fetch_gitleaks_history.py"
        self.assertIn(step, workflow)
        self.assertLess(workflow.index(step), workflow.index("scripts/tests/run.sh --smoke-shard"))


if __name__ == "__main__":
    unittest.main()
