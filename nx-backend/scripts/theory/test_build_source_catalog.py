import json
import copy
import importlib.util
import os
import subprocess
import sys
import tempfile
import unittest
import zipfile
from collections import Counter
from pathlib import Path
from unittest import mock


SCRIPT_DIR = Path(__file__).resolve().parent
SCRIPT = SCRIPT_DIR / "build-source-catalog.py"
SELECTION = SCRIPT_DIR / "round-001-selection.json"
SOURCE_ROOT = Path(
    os.environ.get("THEORY_SOURCE_ROOT", Path.home() / "Desktop" / "韩老师资料" / "芯之力文件")
)
IGNORED_SYSTEM_FILES = {".DS_Store"}


def load_catalog_module():
    spec = importlib.util.spec_from_file_location("build_source_catalog", SCRIPT)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def make_selection(source):
    return {
        "roundId": "round-001",
        "budgetRuleVersion": "xinzhili-page-equivalent-v1",
        "normalizedTextCharactersPerPage": 1800,
        "maxSelectedFiles": 24,
        "maxBudgetPageEquivalent": 2000,
        "maxOcrPageCount": 300,
        "sources": [source],
    }


def make_entry(relative_path="sample.pdf", unit_type="page", unit_count=5, budget=5):
    return {
        "relativePath": relative_path,
        "sha256": "a" * 64,
        "unitEstimate": {
            "unitType": unit_type,
            "unitCount": unit_count,
            "budgetPageEquivalent": budget,
        },
        "extractionRoute": "pdf_text_layer",
    }


def make_source(relative_path="sample.pdf"):
    return {
        "relativePath": relative_path,
        "selectedRanges": ["page:1-2"],
        "processedUnitType": "page",
        "processedUnitCount": 2,
        "budgetPageEquivalent": 2,
        "ocrPageCount": 0,
        "selectionReason": "合成测试来源",
    }


class SelectionValidationUnitTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.module = load_catalog_module()

    def assert_rejected(self, source, entry, message_pattern):
        with self.assertRaisesRegex(ValueError, message_pattern):
            self.module.validate_selection(
                make_selection(source), {entry["relativePath"]: entry}
            )

    def test_rejects_empty_selected_ranges(self):
        source = make_source()
        source["selectedRanges"] = []
        self.assert_rejected(source, make_entry(), "选择范围不能为空")

    def test_rejects_non_positive_processed_unit_count(self):
        for invalid in (0, -1):
            with self.subTest(invalid=invalid):
                source = make_source()
                source["processedUnitCount"] = invalid
                self.assert_rejected(source, make_entry(), "processedUnitCount 必须为正整数")

    def test_rejects_negative_and_non_integer_numeric_fields(self):
        cases = (
            ("processedUnitCount", 1.5, "processedUnitCount 必须为正整数"),
            ("budgetPageEquivalent", -1, "budgetPageEquivalent 必须为非负整数"),
            ("budgetPageEquivalent", 1.5, "budgetPageEquivalent 必须为非负整数"),
            ("ocrPageCount", -1, "ocrPageCount 必须为非负整数"),
            ("ocrPageCount", 1.5, "ocrPageCount 必须为非负整数"),
        )
        for field, invalid, message in cases:
            with self.subTest(field=field, invalid=invalid):
                source = make_source()
                source[field] = invalid
                self.assert_rejected(source, make_entry(), message)

    def test_pdf_budget_must_equal_expanded_selected_pages(self):
        source = make_source()
        source["budgetPageEquivalent"] = 1
        self.assert_rejected(source, make_entry(), "页或幻灯片预算必须等于选定单元数")

    def test_pptx_budget_must_equal_expanded_selected_slides(self):
        entry = make_entry("sample.pptx", "slide", 5, 5)
        entry["extractionRoute"] = "pptx_slide_xml"
        source = make_source("sample.pptx")
        source.update(
            selectedRanges=["slide:1-2"],
            processedUnitType="slide",
            budgetPageEquivalent=3,
        )
        self.assert_rejected(source, entry, "页或幻灯片预算必须等于选定单元数")

    def test_text_budget_must_equal_catalog_probe_estimate(self):
        entry = make_entry("sample.epub", "spine_item", 2, 7)
        entry["unitEstimate"]["normalizedTextCharacters"] = 12600
        entry["extractionRoute"] = "epub_xhtml"
        source = make_source("sample.epub")
        source.update(
            selectedRanges=["spine-item:1-2"],
            processedUnitType="spine_item",
            budgetPageEquivalent=6,
        )
        self.assert_rejected(source, entry, "文本折算预算与目录探测不一致")


class SyntheticCatalogUnitTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.module = load_catalog_module()

    def empty_selection(self):
        selection = make_selection(make_source())
        selection["sources"] = []
        return selection

    def test_probe_error_reason_never_contains_command_or_absolute_path(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            root = Path(temp_dir)
            source = root / "sample.pdf"
            source.write_bytes(b"not-a-pdf")
            secret_path = root / "private" / "document.pdf"
            failure = subprocess.CalledProcessError(
                17, ["pdfinfo", str(secret_path)], stderr=b"private diagnostic"
            )
            with mock.patch.object(self.module, "probe_file", side_effect=failure):
                catalog, _, _ = self.module.build_catalog(root, self.empty_selection())

            entry = catalog["files"][0]
            self.assertEqual("error", entry["catalogStatus"])
            self.assertIn("CalledProcessError", entry["reason"])
            self.assertIn("退出码 17", entry["reason"])
            self.assertNotIn(str(root), entry["reason"])
            self.assertNotIn("pdfinfo", entry["reason"])
            self.assertNotIn("private diagnostic", entry["reason"])

    def test_missing_pdf_tool_only_marks_pdf_error_and_epub_still_builds(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            root = Path(temp_dir)
            (root / "missing-tool.pdf").write_bytes(b"not-a-pdf")
            epub_path = root / "selected.epub"
            with zipfile.ZipFile(epub_path, "w") as archive:
                archive.writestr(
                    "META-INF/container.xml",
                    '<container><rootfiles><rootfile full-path="OPS/package.opf"/></rootfiles></container>',
                )
                archive.writestr(
                    "OPS/package.opf",
                    '<package><manifest><item id="chapter" href="chapter.xhtml"/></manifest>'
                    '<spine><itemref idref="chapter"/></spine></package>',
                )
                archive.writestr("OPS/chapter.xhtml", "<html><body>可提炼内容</body></html>")

            source = make_source("selected.epub")
            source.update(
                selectedRanges=["spine-item:1"],
                processedUnitType="spine_item",
                processedUnitCount=1,
                budgetPageEquivalent=1,
            )
            missing_pdf_source = make_source("missing-tool.pdf")
            missing_pdf_source.update(
                selectedRanges=["page:1"],
                processedUnitCount=1,
                budgetPageEquivalent=1,
            )
            selection = make_selection(source)
            selection["sources"].append(missing_pdf_source)
            original_command_version = self.module.command_version

            def command_version(command):
                if command[0] == "pdfinfo":
                    raise FileNotFoundError("/private/bin/pdfinfo")
                return original_command_version(command)

            with mock.patch.object(self.module, "command_version", side_effect=command_version):
                catalog, _, source_files = self.module.build_catalog(root, selection)

            entries = {item["relativePath"]: item for item in catalog["files"]}
            self.assertEqual("error", entries["missing-tool.pdf"]["catalogStatus"])
            self.assertEqual("selected", entries["selected.epub"]["catalogStatus"])
            self.assertEqual(1, source_files["summary"]["selectedCount"])
            self.assertEqual(["selected.epub"], [item["relativePath"] for item in source_files["files"]])


class BuildSourceCatalogTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.temp_dir = tempfile.TemporaryDirectory()
        cls.output_root = Path(cls.temp_dir.name) / "data" / "theory" / "xinzhili"
        subprocess.run(
            [
                sys.executable,
                str(SCRIPT),
                "--source-root",
                str(SOURCE_ROOT),
                "--selection",
                str(SELECTION),
                "--output-root",
                str(cls.output_root),
            ],
            check=True,
            text=True,
            capture_output=True,
        )
        cls.catalog = json.loads((cls.output_root / "source-catalog.json").read_text())
        round_catalog = cls.output_root / "round-001" / "catalog"
        cls.works = json.loads((round_catalog / "works.json").read_text())
        cls.source_files = json.loads((round_catalog / "source-files.json").read_text())
        cls.selection = json.loads(SELECTION.read_text())
        cls.module = load_catalog_module()
        cls.entries_by_path = {item["relativePath"]: item for item in cls.catalog["files"]}

    @classmethod
    def tearDownClass(cls):
        cls.temp_dir.cleanup()

    def test_catalog_accounts_for_every_physical_content_file_dynamically(self):
        actual_paths = {
            path.relative_to(SOURCE_ROOT).as_posix()
            for path in SOURCE_ROOT.rglob("*")
            if path.is_file() and path.name not in IGNORED_SYSTEM_FILES
        }
        entries = self.catalog["files"]
        self.assertEqual(actual_paths, {entry["relativePath"] for entry in entries})

        statuses = Counter(entry["catalogStatus"] for entry in entries)
        self.assertEqual(
            len(actual_paths),
            sum(statuses[name] for name in ("selected", "backlog", "duplicate", "excluded", "error")),
        )
        self.assertEqual(self.catalog["summary"]["statusCounts"], dict(sorted(statuses.items())))

    def test_every_entry_has_traceable_catalog_metadata(self):
        required = {
            "relativePath",
            "sha256",
            "catalogStatus",
            "canonicalWorkId",
            "duplicateGroupId",
            "extractionRoute",
            "unitEstimate",
            "ocrPageEstimate",
            "reason",
            "priority",
            "proposedBatch",
            "probe",
        }
        for entry in self.catalog["files"]:
            self.assertTrue(required.issubset(entry), entry["relativePath"])
            self.assertNotIn(str(SOURCE_ROOT), json.dumps(entry, ensure_ascii=False))
            self.assertRegex(entry["probe"]["outputSha256"], r"^[0-9a-f]{64}$")
            self.assertTrue(entry["probe"]["toolVersions"])
            if entry["catalogStatus"] == "backlog":
                self.assertTrue(entry["extractionRoute"])
                self.assertRegex(entry["proposedBatch"], r"^round-\d{3}$")
                self.assertTrue(entry["reason"])

    def test_epub_manifest_url_encoding_resolves_to_real_spine_members(self):
        encoded_href_epub = next(
            entry for entry in self.catalog["files"]
            if entry["relativePath"].startswith("认知觉醒：开启自我改变的原动力")
        )
        self.assertNotEqual("error", encoded_href_epub["catalogStatus"])
        self.assertEqual("epub_xhtml", encoded_href_epub["extractionRoute"])

    def test_round_one_selection_obeys_count_budget_ocr_and_dedup_limits(self):
        selected = self.source_files["files"]
        self.assertLessEqual(len(selected), 24)
        self.assertEqual(len(selected), len({item["sha256"] for item in selected}))
        self.assertLessEqual(sum(item["budgetPageEquivalent"] for item in selected), 2000)
        self.assertLessEqual(sum(item["ocrPageCount"] for item in selected), 300)
        self.assertEqual(
            self.source_files["summary"]["selectedCount"],
            len(selected),
        )
        self.assertEqual(
            self.source_files["summary"]["budgetPageEquivalent"],
            sum(item["budgetPageEquivalent"] for item in selected),
        )
        self.assertEqual(
            self.source_files["summary"]["ocrPageCount"],
            sum(item["ocrPageCount"] for item in selected),
        )

    def test_selected_sources_have_real_paths_ranges_and_unit_contracts(self):
        required_candidates = {
            *(f"能量/{index:02d}" for index in range(1, 13)),
            "九型理论基础/九型人格·珍藏版.epub",
            "(NEW)周易.pdf",
            "周易说卦传正解.doc",
            "《易经》中蕴含人生大智慧的30个成语.docx",
            "英雄之旅.pdf",
            "《千面英雄》-约瑟夫-坎贝尔.pdf",
            "《李中莹NLP精义》（李中莹）.pdf",
            "《成功母亲教育：亲子关系全面技巧》（李中莹）.pdf",
            "重塑心灵-李中莹.pdf",
            "《NLP简快心理疗法》李中莹.pdf",
            "情绪的语言.pdf",
            "非暴力沟通 (卢森堡) (Z-Library).epub",
        }
        selected_paths = {item["relativePath"] for item in self.source_files["files"]}
        for expected in required_candidates:
            self.assertTrue(
                any(path == expected or path.startswith(expected) for path in selected_paths),
                expected,
            )

        for item in self.source_files["files"]:
            self.assertTrue((SOURCE_ROOT / item["relativePath"]).is_file())
            self.assertTrue(item["selectedRanges"])
            self.assertIn(item["processedUnitType"], {"page", "spine_item", "paragraph"})
            self.assertGreater(item["processedUnitCount"], 0)
            self.assertGreater(item["budgetPageEquivalent"], 0)
            self.assertGreaterEqual(item["ocrPageCount"], 0)
            self.assertIn("remainingRanges", item)
            self.assertIn("remainingUnitCount", item)
            self.assertIn("proposedBatch", item)

        office_sources = {
            item["relativePath"]: item
            for item in self.source_files["files"]
            if Path(item["relativePath"]).suffix.lower() in {".doc", ".docx"}
        }
        for item in office_sources.values():
            self.assertEqual("paragraph", item["processedUnitType"])
            self.assertTrue(all(value.startswith("paragraph:") for value in item["selectedRanges"]))

        epub_sources = [
            item for item in self.source_files["files"] if item["mediaType"] == "epub"
        ]
        for item in epub_sources:
            self.assertTrue(all(value.startswith("spine-item:") for value in item["selectedRanges"]))

    def test_duplicate_files_are_grouped_and_never_selected_twice(self):
        duplicate_groups = {}
        for item in self.catalog["files"]:
            if item["duplicateGroupId"]:
                duplicate_groups.setdefault(item["duplicateGroupId"], []).append(item)
        self.assertTrue(duplicate_groups)
        for group in duplicate_groups.values():
            self.assertGreaterEqual(len(group), 2)
            self.assertEqual(1, len({item["canonicalWorkId"] for item in group}))
            self.assertLessEqual(sum(item["catalogStatus"] == "selected" for item in group), 1)

        work_ids = {item["workId"] for item in self.works["works"]}
        self.assertEqual(work_ids, {item["canonicalWorkId"] for item in self.source_files["files"]})

    def assert_selection_rejected(self, mutate, message_pattern):
        selection = copy.deepcopy(self.selection)
        mutate(selection)
        with self.assertRaisesRegex(ValueError, message_pattern):
            self.module.validate_selection(selection, self.entries_by_path)

    def test_rejects_more_than_24_sources(self):
        backlog = next(
            item for item in self.catalog["files"]
            if item["catalogStatus"] == "backlog" and item["unitEstimate"]["unitCount"] > 0
        )

        def mutate(selection):
            selection["sources"].append({
                "relativePath": backlog["relativePath"],
                "selectedRanges": ["page:1"],
                "processedUnitType": "page",
                "processedUnitCount": 1,
                "budgetPageEquivalent": 1,
                "ocrPageCount": 0,
            })

        self.assert_selection_rejected(mutate, "文件数超过上限")

    def test_rejects_page_and_ocr_budget_overruns(self):
        self.assert_selection_rejected(
            lambda selection: selection["sources"][0].update(budgetPageEquivalent=2001),
            "处理页折算超过上限",
        )
        self.assert_selection_rejected(
            lambda selection: selection["sources"][0].update(ocrPageCount=301),
            "OCR 页数超过上限",
        )

    def test_rejects_duplicate_sha_in_selection(self):
        duplicate_group = next(
            group_id for group_id, count in Counter(
                item["duplicateGroupId"] for item in self.catalog["files"] if item["duplicateGroupId"]
            ).items() if count >= 2
        )
        duplicates = [
            item for item in self.catalog["files"] if item["duplicateGroupId"] == duplicate_group
        ]

        def mutate(selection):
            for index, duplicate in enumerate(duplicates[:2]):
                selection["sources"][index] = {
                    "relativePath": duplicate["relativePath"],
                    "selectedRanges": ["page:1"],
                    "processedUnitType": "page",
                    "processedUnitCount": 1,
                    "budgetPageEquivalent": 1,
                    "ocrPageCount": 0,
                }

        self.assert_selection_rejected(mutate, "SHA-256 重复")

    def test_rejects_invalid_out_of_bounds_and_mismatched_ranges(self):
        self.assert_selection_rejected(
            lambda selection: selection["sources"][0].update(selectedRanges=["page:6-1"]),
            "范围起止非法",
        )
        self.assert_selection_rejected(
            lambda selection: selection["sources"][0].update(selectedRanges=["page:1-99"]),
            "选择范围越界",
        )
        self.assert_selection_rejected(
            lambda selection: selection["sources"][0].update(selectedRanges=["page:1-5"]),
            "展开数量.*processedUnitCount",
        )
        self.assert_selection_rejected(
            lambda selection: selection["sources"][0].update(processedUnitType="paragraph"),
            "单元类型与格式不匹配",
        )

    def test_rejects_ocr_route_and_selected_page_mismatch(self):
        scanned_index = next(
            index for index, item in enumerate(self.selection["sources"])
            if item["relativePath"] == "情绪的语言.pdf"
        )
        self.assert_selection_rejected(
            lambda selection: selection["sources"][scanned_index].update(ocrPageCount=79),
            "OCR 页数必须与选定页数一致",
        )


if __name__ == "__main__":
    unittest.main()
