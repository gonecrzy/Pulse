"""The personal fork validates only its own development branches."""
from pathlib import Path
import re
import unittest

ROOT = Path(__file__).resolve().parents[2]


class ForkBranchCIContractTest(unittest.TestCase):
    def test_build_and_e2e_cover_fork_branches_without_upstream_release_branches(self):
        for workflow in ("build-and-test.yml", "test-e2e.yml"):
            source = (ROOT / ".github/workflows" / workflow).read_text()
            for event in ("push", "pull_request"):
                with self.subTest(workflow=workflow, event=event):
                    block = re.search(
                        rf"^  {event}:\n(.*?)(?=^  \w|^\S|\Z)",
                        source, re.MULTILINE | re.DOTALL,
                    )
                    self.assertIsNotNone(block)
                    branches = re.search(
                        r"^    branches:\n((?:      - .*\n)+)",
                        block.group(1), re.MULTILINE,
                    )
                    self.assertIsNotNone(branches)
                    patterns = [line.strip()[2:].strip('\"\'')
                                for line in branches.group(1).splitlines()]
                    self.assertEqual(patterns, ["main", "sindri", "feature/sindri-integration"])
                    self.assertFalse(any(pattern.startswith("release/") for pattern in patterns))


if __name__ == "__main__":
    unittest.main()
