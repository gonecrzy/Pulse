#!/usr/bin/env python3
"""Fetch only historical Git objects referenced by .gitleaksignore.

The fork does not retain every commit from the upstream history. These exact
public commit objects are test data for fingerprint resolution only: this
script never checks out, imports, or executes fetched application code.
"""

from __future__ import annotations

import os
import re
import subprocess
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
IGNORE_PATH = ROOT / ".gitleaksignore"
UPSTREAM_URL = "https://github.com/rcourtman/Pulse.git"
HISTORICAL_FINGERPRINT = re.compile(
    r"^(?P<commit>[0-9a-f]{40}):(?P<path>.+):(?P<rule>[a-z0-9-]+):(?P<line>[1-9][0-9]*)$"
)


def fetch_missing_objects(
    root: Path = ROOT,
    ignore_path: Path = IGNORE_PATH,
    runner_temp: Path | None = None,
    github_env: Path | None = None,
) -> int:
    """Fetch missing fingerprint objects into an isolated alternate object DB."""
    entries = [
        line.strip()
        for line in ignore_path.read_text(encoding="utf-8").splitlines()
        if line.strip() and not line.lstrip().startswith("#")
    ]
    historical = [match for entry in entries if (match := HISTORICAL_FINGERPRINT.fullmatch(entry))]
    commits = sorted({match.group("commit") for match in historical})
    missing = [
        commit for commit in commits
        if subprocess.run(
            ["git", "cat-file", "-e", f"{commit}^{{commit}}"],
            cwd=root,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            check=False,
        ).returncode != 0
    ]
    if not missing:
        return 0

    if runner_temp is None or github_env is None:
        raise RuntimeError("RUNNER_TEMP and GITHUB_ENV are required to isolate fetched history")
    object_repo = runner_temp / "gitleaks-history.git"
    if object_repo.exists():
        raise RuntimeError("isolated gitleaks history repository already exists")
    subprocess.run(["git", "init", "--bare", str(object_repo)], cwd=root, check=True)
    git_object_db = ["git", "--git-dir", str(object_repo)]
    if missing:
        subprocess.run(
            [*git_object_db, "fetch", "--no-tags", "--depth=1", UPSTREAM_URL, *missing],
            cwd=root,
            check=True,
        )

    unresolved = []
    verify_env = {**os.environ, "GIT_ALTERNATE_OBJECT_DIRECTORIES": str(object_repo / "objects")}
    for match in historical:
        object_name = f"{match.group('commit')}:{match.group('path')}"
        result = subprocess.run(
            ["git", "cat-file", "-e", object_name],
            cwd=root,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            check=False,
            env=verify_env,
        )
        if result.returncode != 0:
            unresolved.append(object_name)
    if unresolved:
        raise RuntimeError(f"{len(unresolved)} historical gitleaks fingerprint object(s) remain unavailable")
    with github_env.open("a", encoding="utf-8") as stream:
        stream.write(f"GIT_ALTERNATE_OBJECT_DIRECTORIES={object_repo.resolve() / 'objects'}\n")
    return len(missing)


if __name__ == "__main__":
    runner_temp = Path(os.environ["RUNNER_TEMP"])
    github_env = Path(os.environ["GITHUB_ENV"])
    fetched = fetch_missing_objects(runner_temp=runner_temp, github_env=github_env)
    print(f"Verified historical gitleaks fingerprint objects; fetched {fetched} missing commit(s).")
