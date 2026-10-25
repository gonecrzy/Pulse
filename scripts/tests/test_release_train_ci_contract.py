"""The personal fork validates only its own development branches."""
from pathlib import Path
import re
import unittest

ROOT = Path(__file__).resolve().parents[2]


class ForkBranchCIContractTest(unittest.TestCase):
    def test_validation_runs_on_sindri_prs_and_build_is_reusable(self):
        for workflow in ("build-and-test.yml", "test-e2e.yml"):
            source = (ROOT / ".github/workflows" / workflow).read_text()
            block = re.search(
                r"^  pull_request:\n(.*?)(?=^  \w|^\S|\Z)",
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
            self.assertEqual(patterns, ["sindri"])
            self.assertNotRegex(source, r"(?m)^  push:\s*$")
        build = (ROOT / ".github/workflows/build-and-test.yml").read_text()
        e2e = (ROOT / ".github/workflows/test-e2e.yml").read_text()
        self.assertRegex(build, r"(?m)^  workflow_call:\s*$")
        self.assertRegex(e2e, r"(?m)^  workflow_dispatch:\s*$")


if __name__ == "__main__":
    unittest.main()
