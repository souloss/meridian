import json
import unittest
from pathlib import Path


REPOSITORY_ROOT = Path(__file__).resolve().parents[1]


class FrontendToolingTest(unittest.TestCase):
    def test_api_generation_prepares_nuxt_before_orval(self):
        package = json.loads((REPOSITORY_ROOT / "web" / "package.json").read_text())
        command = package["scripts"]["generate:api"].split("&&")
        steps = [step.strip() for step in command]

        self.assertIn("nuxt prepare", steps)
        self.assertIn("orval --config orval.config.ts", steps)
        self.assertLess(
            steps.index("nuxt prepare"),
            steps.index("orval --config orval.config.ts"),
        )


if __name__ == "__main__":
    unittest.main()
