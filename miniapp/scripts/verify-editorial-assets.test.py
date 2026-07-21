#!/usr/bin/env python3
"""Regression tests for the editorial asset verifier."""

from __future__ import annotations

import contextlib
import importlib.util
import io
import shutil
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

from PIL import Image


SCRIPT_PATH = Path(__file__).with_name("verify-editorial-assets.py")
SPEC = importlib.util.spec_from_file_location("verify_editorial_assets", SCRIPT_PATH)
assert SPEC is not None and SPEC.loader is not None
VERIFIER = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = VERIFIER
SPEC.loader.exec_module(VERIFIER)
SOURCE_ASSET_DIR = VERIFIER.ASSET_DIR


class EditorialAssetVerifierTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temp_dir = tempfile.TemporaryDirectory()
        self.root = Path(self.temp_dir.name)
        self.asset_dir = self.root / "src" / "static" / "editorial"
        self.asset_dir.mkdir(parents=True)
        for name in VERIFIER.INITIAL_ASSETS:
            shutil.copy2(SOURCE_ASSET_DIR / name, self.asset_dir / name)

    def tearDown(self) -> None:
        self.temp_dir.cleanup()

    def run_verifier(self, group: str) -> tuple[int, str]:
        output = io.StringIO()
        with (
            mock.patch.object(VERIFIER, "ROOT", self.root),
            mock.patch.object(VERIFIER, "ASSET_DIR", self.asset_dir),
            mock.patch.object(sys, "argv", [str(SCRIPT_PATH), "--group", group]),
            contextlib.redirect_stdout(output),
        ):
            return VERIFIER.main(), output.getvalue()

    def add_valid_result_assets(self) -> None:
        for name, spec in VERIFIER.RESULT_ASSETS.items():
            Image.new("RGB", (spec.width, spec.height), "white").save(
                self.asset_dir / name,
                "WEBP",
            )

    def test_initial_allows_known_result_assets_to_coexist(self) -> None:
        for name in VERIFIER.RESULT_ASSETS:
            (self.asset_dir / name).touch()

        return_code, output = self.run_verifier("initial")

        self.assertEqual(0, return_code, output)

    def test_initial_rejects_unknown_files_of_any_type(self) -> None:
        for name in ("stray.png", "draft.jpg", "asset.tmp", ".DS_Store"):
            (self.asset_dir / name).touch()

        return_code, output = self.run_verifier("initial")

        self.assertEqual(1, return_code, output)
        for name in ("stray.png", "draft.jpg", "asset.tmp", ".DS_Store"):
            self.assertIn(name, output)

    def test_all_validates_all_sixteen_assets(self) -> None:
        self.add_valid_result_assets()

        return_code, output = self.run_verifier("all")

        self.assertEqual(0, return_code, output)
        self.assertIn("16 files verified", output)


if __name__ == "__main__":
    unittest.main()
