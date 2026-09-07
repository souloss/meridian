import importlib.util
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch


MODULE_PATH = Path(__file__).with_name("sync-openapi.py")
SPEC = importlib.util.spec_from_file_location("sync_openapi", MODULE_PATH)
assert SPEC is not None and SPEC.loader is not None
sync_openapi = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(sync_openapi)


class CheckOutputsTest(unittest.TestCase):
    def test_materializes_ignored_projections_but_checks_canonical_output(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            canonical = root / "contracts" / "openapi.yaml"
            projection = root / "build" / "contracts" / "api" / "common" / "openapi.yaml"
            canonical.parent.mkdir(parents=True)
            canonical.write_text("canonical\n", encoding="utf-8")

            with patch.object(sync_openapi, "REPOSITORY_ROOT", root):
                result = sync_openapi.check_outputs(
                    {canonical: "canonical\n", projection: "projection\n"},
                    materialize={projection},
                )

            self.assertEqual(result, 0)
            self.assertEqual(projection.read_text(encoding="utf-8"), "projection\n")

    def test_canonical_drift_still_fails(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            canonical = Path(directory) / "contracts" / "openapi.yaml"
            canonical.parent.mkdir(parents=True)
            canonical.write_text("stale\n", encoding="utf-8")

            with patch.object(sync_openapi, "REPOSITORY_ROOT", Path(directory)):
                result = sync_openapi.check_outputs({canonical: "generated\n"}, materialize=set())

            self.assertEqual(result, 1)


if __name__ == "__main__":
    unittest.main()
