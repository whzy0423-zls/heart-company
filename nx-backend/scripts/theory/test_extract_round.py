import base64
import hashlib
import importlib.util
import json
import os
import tempfile
import unittest
import zipfile
from pathlib import Path
from unittest import mock


SCRIPT_DIR = Path(__file__).resolve().parent
SCRIPT = SCRIPT_DIR / "extract-round.py"
PNG_1X1 = base64.b64decode(
    "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="
)


def load_module():
    spec = importlib.util.spec_from_file_location("extract_round", SCRIPT)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def sha256_bytes(payload):
    return hashlib.sha256(payload).hexdigest()


def make_selection(relative_path="sample.pdf", sha256=None, ocr_pages=0):
    return {
        "roundId": "round-001",
        "budgetRuleVersion": "xinzhili-page-equivalent-v1",
        "normalizedTextCharactersPerPage": 1800,
        "maxSelectedFiles": 24,
        "maxBudgetPageEquivalent": 2000,
        "maxOcrPageCount": 300,
        "sources": [
            {
                "relativePath": relative_path,
                "selectedRanges": ["page:1-2"],
                "processedUnitType": "page",
                "processedUnitCount": 2,
                "budgetPageEquivalent": 2,
                "ocrPageCount": ocr_pages,
                "selectionReason": "合成测试",
            }
        ],
    }


def make_catalog(relative_path="sample.pdf", sha256="a" * 64, route="pdf_text_layer"):
    return {
        "schemaVersion": "xinzhili-source-catalog-v1",
        "files": [
            {
                "relativePath": relative_path,
                "sha256": sha256,
                "catalogStatus": "selected",
                "extractionRoute": route,
                "unitEstimate": {
                    "unitType": "page",
                    "unitCount": 2,
                    "budgetPageEquivalent": 2,
                },
            }
        ],
    }


def write_valid_pdf_output(root, module):
    source = make_selection()["sources"][0]
    entry = make_catalog()["files"][0]
    units = []
    for page, text in ((1, "第一页"), (2, "第二页")):
        units.append(module.write_text_unit(
            root, f"units/page-{page:04d}-slice-01.txt", text,
            {"page": page, "slice": 1, "characterStart": 0, "characterEnd": len(text)}, 1.0,
        ))
    qa = Path(root) / "qa"
    qa.mkdir(parents=True)
    (qa / "cover.png").write_bytes(PNG_1X1)
    (qa / "selected-page.png").write_bytes(PNG_1X1)
    automated_checks = module.inspect_png(qa / "cover.png")
    quality = {
        "status": "pending_human_review",
        "scope": module.PDF_QUALITY_SCOPE,
        "selectedPages": 2,
        "extractedPages": 2,
        "emptyTextPages": 0,
        "emptyTextUnits": 0,
        "renderCount": 2,
        "qaSummary": {
            "blackFailures": 2, "blankFailures": 0, "edgeCropWarnings": 2,
        },
        "renders": [
            {"kind": "cover", "page": 1, "imageFile": "qa/cover.png",
             "imageSha256": sha256_bytes(PNG_1X1), "automatedChecks": automated_checks},
            {"kind": "selected-page", "page": 2, "imageFile": "qa/selected-page.png",
             "imageSha256": sha256_bytes(PNG_1X1), "automatedChecks": automated_checks},
        ],
        "extractionQuality": module.summarize_extraction_quality(units),
    }
    manifest = {
        "schemaVersion": module.SCHEMA_VERSION, "status": "complete", "roundId": "round-001",
        "relativePath": source["relativePath"], "sourceSha256": entry["sha256"],
        "extractionRoute": entry["extractionRoute"], "selectedRanges": source["selectedRanges"],
        "processedUnitType": "page", "processedUnitCount": 2,
        "budgetPageEquivalent": 2, "ocrPageCount": 0, "tools": {}, "parameters": [],
        "units": units, "qualityReport": "quality.json", "errorReport": "errors.json",
    }
    module.write_json(Path(root) / "manifest.json", manifest)
    module.write_json(Path(root) / "quality.json", quality)
    module.write_json(Path(root) / "errors.json", {"status": "complete", "errors": []})
    return source, entry, manifest, quality


def write_valid_non_pdf_output(root, module, unit_type):
    if unit_type == "spine_item":
        relative_path, route, selected_ranges = "sample.epub", "epub_xhtml", ["spine-item:1-2"]
        locator_values = [
            {"spineItem": 1, "chapter": "第一章", "paragraph": 1},
            {"spineItem": 2, "chapter": "第二章", "paragraph": 1},
        ]
        scope = module.EPUB_QUALITY_SCOPE
    else:
        relative_path, route, selected_ranges = "sample.docx", "textutil_plain_text", ["paragraph:1-2"]
        locator_values = [{"heading": "第一章", "paragraph": 1}, {"heading": "第一章", "paragraph": 2}]
        scope = module.OFFICE_QUALITY_SCOPE
    source = {
        "relativePath": relative_path, "selectedRanges": selected_ranges,
        "processedUnitType": unit_type, "processedUnitCount": 2,
        "budgetPageEquivalent": 1, "ocrPageCount": 0, "selectionReason": "测试",
    }
    entry = {"relativePath": relative_path, "sha256": "b" * 64, "catalogStatus": "selected",
             "extractionRoute": route, "unitEstimate": {"unitType": unit_type, "unitCount": 2,
                                                          "budgetPageEquivalent": 1}}
    units = [module.write_text_unit(root, f"units/unit-{index}.txt", f"文本{index}", locator, 1.0)
             for index, locator in enumerate(locator_values, start=1)]
    quality = {"status": "pending_human_review", "scope": scope,
               "extractedParagraphs": 2, "emptyTextUnits": 0,
               "extractionQuality": module.summarize_extraction_quality(units)}
    if unit_type == "spine_item":
        quality["selectedSpineItems"] = 2
    else:
        quality["selectedParagraphs"] = 2
    manifest = {
        "schemaVersion": module.SCHEMA_VERSION, "status": "complete", "roundId": "round-001",
        "relativePath": relative_path, "sourceSha256": entry["sha256"], "extractionRoute": route,
        "selectedRanges": selected_ranges, "processedUnitType": unit_type, "processedUnitCount": 2,
        "budgetPageEquivalent": 1, "ocrPageCount": 0, "tools": {}, "parameters": [],
        "units": units, "qualityReport": "quality.json", "errorReport": "errors.json",
    }
    if unit_type == "spine_item":
        manifest["semanticBlockContract"] = "v1"
    module.write_json(Path(root) / "manifest.json", manifest)
    module.write_json(Path(root) / "quality.json", quality)
    module.write_json(Path(root) / "errors.json", {"status": "complete", "errors": []})
    return source, entry, manifest, quality


class InputContractTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.module = load_module()

    def test_real_selection_has_expected_bounded_totals(self):
        selection = json.loads((SCRIPT_DIR / "round-001-selection.json").read_text("utf-8"))
        totals = self.module.selection_totals(selection)
        self.assertEqual(24, totals["sourceCount"])
        self.assertEqual(1435, totals["budgetPageEquivalent"])
        self.assertEqual(257, totals["ocrPageCount"])
        self.assertLessEqual(totals["budgetPageEquivalent"], 2000)
        self.assertLessEqual(totals["ocrPageCount"], 300)

    def test_rejects_source_sha_mismatch_before_extraction(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            root = Path(temp_dir)
            (root / "sample.pdf").write_bytes(b"actual")
            selection = make_selection()
            catalog = make_catalog(sha256=sha256_bytes(b"different"))
            with self.assertRaisesRegex(ValueError, "SHA-256"):
                self.module.validate_inputs(selection, catalog, root)

    def test_rejects_range_count_or_budget_drift(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            root = Path(temp_dir)
            payload = b"actual"
            (root / "sample.pdf").write_bytes(payload)
            selection = make_selection()
            selection["sources"][0]["processedUnitCount"] = 1
            catalog = make_catalog(sha256=sha256_bytes(payload))
            with self.assertRaisesRegex(ValueError, "selectedRanges"):
                self.module.validate_inputs(selection, catalog, root)

    def test_missing_required_tool_or_language_fails_closed(self):
        with mock.patch.object(self.module.shutil, "which", return_value=None):
            with self.assertRaisesRegex(RuntimeError, "缺少必需工具"):
                self.module.require_runtime(needs_ocr=True, needs_office=True)


class LocatorAndManifestTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.module = load_module()

    def test_long_pdf_page_is_split_with_reviewable_locator(self):
        text = "甲" * 9001
        slices = self.module.slice_pdf_page(7, text, 4000)
        self.assertEqual(text, "".join(item["text"] for item in slices))
        self.assertEqual([1, 2, 3], [item["locator"]["slice"] for item in slices])
        self.assertTrue(all(item["locator"]["page"] == 7 for item in slices))
        self.assertEqual(0, slices[0]["locator"]["characterStart"])
        self.assertEqual(len(text), slices[-1]["locator"]["characterEnd"])

    def test_ocr_dpi_keeps_ultra_tall_course_pages_below_tesseract_limit(self):
        self.assertEqual(150, self.module.OCR_DPI)

    def test_office_locator_uses_heading_and_paragraph_not_fake_page(self):
        units = self.module.office_units("第一章\n第一段内容。\n第二段内容。", {1, 2, 3})
        self.assertEqual(3, len(units))
        self.assertEqual(1, units[0]["locator"]["paragraph"])
        self.assertEqual("第一章", units[1]["locator"]["heading"])
        self.assertNotIn("page", units[1]["locator"])

    def test_written_unit_is_utf8_and_manifest_hashes_match(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            root = Path(temp_dir)
            record = self.module.write_text_unit(
                root, "units/page-0001-slice-01.txt", "可追溯文本", {"page": 1, "slice": 1}, 0.875
            )
            payload = (root / record["textFile"]).read_bytes()
            self.assertEqual("可追溯文本", payload.decode("utf-8"))
            self.assertEqual(sha256_bytes(payload), record["textSha256"])
            self.assertEqual(len("可追溯文本"), record["characterCount"])
            self.assertEqual(0.875, record["confidence"])
            empty = self.module.write_text_unit(root, "units/empty.txt", "", {"page": 2}, 0.0)
            self.assertTrue(empty["emptyText"])

    def test_manifest_validator_rejects_missing_or_untraceable_fields(self):
        manifest = {
            "schemaVersion": self.module.SCHEMA_VERSION, "status": "complete", "roundId": "round-001",
            "relativePath": "sample.pdf", "extractionRoute": "pdf_text_layer",
            "selectedRanges": ["page:1"], "processedUnitType": "page",
            "sourceSha256": "a" * 64,
            "processedUnitCount": 1,
            "budgetPageEquivalent": 1,
            "ocrPageCount": 0,
            "tools": {}, "parameters": [], "qualityReport": "quality.json", "errorReport": "errors.json",
            "units": [{"locator": {}, "characterCount": 0, "confidence": 1}],
        }
        with self.assertRaisesRegex(ValueError, "locator"):
            self.module.validate_source_manifest(manifest, "page")

    def test_distinct_unit_count_only_reads_the_active_locator_contract(self):
        records = [{"locator": {"page": 1}}, {"locator": {"page": 1}}, {"locator": {"page": 2}}]
        self.assertEqual(2, self.module.distinct_unit_count(records, "page"))

    def test_extraction_quality_reports_empty_text_and_confidence(self):
        records = [{"characterCount": 0, "confidence": 0.0}, {"characterCount": 3, "confidence": 0.8}]
        self.assertEqual(
            {"unitFileCount": 2, "emptyTextUnitCount": 1, "minimumConfidence": 0.0,
             "averageConfidence": 0.4, "characterCount": 3},
            self.module.summarize_extraction_quality(records),
        )


class EpubSafetyTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.module = load_module()

    def test_epub_rejects_zip_slip(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            path = Path(temp_dir) / "bad.epub"
            with zipfile.ZipFile(path, "w") as archive:
                archive.writestr("../escape.xhtml", "<p>bad</p>")
            with self.assertRaisesRegex(ValueError, "不安全路径"):
                self.module.read_epub_spine(path)

    def test_epub_rejects_compression_bomb_ratio(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            path = Path(temp_dir) / "bomb.epub"
            with zipfile.ZipFile(path, "w", zipfile.ZIP_DEFLATED) as archive:
                archive.writestr("huge.xhtml", "甲" * 2_000_000)
            with self.assertRaisesRegex(ValueError, "压缩比"):
                self.module.read_epub_spine(path, max_compression_ratio=20)

    def test_epub_uses_spine_chapter_paragraph_locator(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            path = Path(temp_dir) / "book.epub"
            with zipfile.ZipFile(path, "w") as archive:
                archive.writestr(
                    "META-INF/container.xml",
                    '<container><rootfiles><rootfile full-path="OPS/book.opf"/></rootfiles></container>',
                )
                archive.writestr(
                    "OPS/book.opf",
                    '<package><manifest><item id="c1" href="c1.xhtml" media-type="application/xhtml+xml"/>'
                    '</manifest><spine><itemref idref="c1"/></spine></package>',
                )
                archive.writestr(
                    "OPS/c1.xhtml",
                    '<html xmlns="http://www.w3.org/1999/xhtml"><body><h1>开篇</h1><p>第一段</p><p>第二段</p></body></html>',
                )
            units = self.module.epub_units(path, {1})
            self.assertEqual(["开篇", "第一段", "第二段"], [unit["text"] for unit in units])
            self.assertEqual(
                {"spineItem": 1, "chapter": "开篇", "paragraph": 1}, units[0]["locator"]
            )
            self.assertNotIn("page", units[0]["locator"])

    def test_epub_rejects_malformed_xhtml(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            path = Path(temp_dir) / "bad.epub"
            with zipfile.ZipFile(path, "w") as archive:
                archive.writestr(
                    "META-INF/container.xml",
                    '<container><rootfiles><rootfile full-path="OPS/book.opf"/></rootfiles></container>',
                )
                archive.writestr(
                    "OPS/book.opf",
                    '<package><manifest><item id="c1" href="c1.xhtml"/></manifest>'
                    '<spine><itemref idref="c1"/></spine></package>',
                )
                archive.writestr("OPS/c1.xhtml", "<html><body><p>broken")
            with self.assertRaisesRegex(ValueError, "XHTML"):
                self.module.epub_units(path, {1})

    def test_epub_cover_uses_image_alt_as_traceable_paragraph(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            path = Path(temp_dir) / "cover.epub"
            with zipfile.ZipFile(path, "w") as archive:
                archive.writestr("META-INF/container.xml", '<container><rootfiles><rootfile full-path="OPS/book.opf"/></rootfiles></container>')
                archive.writestr("OPS/book.opf", '<package><manifest><item id="c" href="c.xhtml"/></manifest><spine><itemref idref="c"/></spine></package>')
                archive.writestr("OPS/c.xhtml", '<html><head><title>封面</title></head><body><img alt="cover"/></body></html>')
            units = self.module.epub_units(path, {1})
            self.assertEqual("cover", units[0]["text"])
            self.assertEqual({"spineItem": 1, "chapter": "封面", "paragraph": 1}, units[0]["locator"])

    def test_epub_svg_cover_falls_back_to_document_title(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            path = Path(temp_dir) / "svg-cover.epub"
            with zipfile.ZipFile(path, "w") as archive:
                archive.writestr("META-INF/container.xml", '<container><rootfiles><rootfile full-path="book.opf"/></rootfiles></container>')
                archive.writestr("book.opf", '<package><manifest><item id="c" href="cover.xhtml"/></manifest><spine><itemref idref="c"/></spine></package>')
                archive.writestr("cover.xhtml", '<html><head><title>Cover</title></head><body><svg><image href="cover.jpg"/></svg></body></html>')
            units = self.module.epub_units(path, {1})
            self.assertEqual("Cover", units[0]["text"])
            self.assertEqual(1, units[0]["locator"]["paragraph"])

    def test_epub_toc_uses_leaf_div_text_without_duplicates(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            path = Path(temp_dir) / "toc.epub"
            with zipfile.ZipFile(path, "w") as archive:
                archive.writestr("META-INF/container.xml", '<container><rootfiles><rootfile full-path="book.opf"/></rootfiles></container>')
                archive.writestr("book.opf", '<package><manifest><item id="c" href="toc.xhtml"/></manifest><spine><itemref idref="c"/></spine></package>')
                archive.writestr("toc.xhtml", '<html><head><title>目录</title></head><body><div><a>第一章</a></div><div><a>第二章</a></div></body></html>')
            units = self.module.epub_units(path, {1})
            self.assertEqual(["第一章", "第二章"], [unit["text"] for unit in units])

    def test_epub_legal_empty_spine_emits_explicit_empty_text_unit(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            path = Path(temp_dir) / "empty-cover.epub"
            with zipfile.ZipFile(path, "w") as archive:
                archive.writestr("META-INF/container.xml", '<container><rootfiles><rootfile full-path="book.opf"/></rootfiles></container>')
                archive.writestr("book.opf", '<package><manifest><item id="c" href="cover.xhtml"/></manifest><spine><itemref idref="c"/></spine></package>')
                archive.writestr("cover.xhtml", '<html><head></head><body><svg><image href="cover.jpg"/></svg></body></html>')
            units = self.module.epub_units(path, {1})
            self.assertEqual("", units[0]["text"])
            self.assertTrue(units[0]["emptyText"])
            self.assertEqual({"spineItem": 1, "chapter": "spine-0001", "paragraph": 1}, units[0]["locator"])


class AtomicOutputTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.module = load_module()

    def test_failed_source_never_leaves_complete_manifest(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            output = Path(temp_dir) / "source"
            with self.assertRaisesRegex(RuntimeError, "boom"):
                self.module.atomic_source_output(
                    output,
                    lambda staging: (_ for _ in ()).throw(RuntimeError("boom")),
                )
            self.assertFalse((output / "manifest.json").exists())
            failure = json.loads((output / "errors.json").read_text("utf-8"))
            self.assertEqual("failed", failure["status"])
            self.assertNotIn(str(Path(temp_dir)), json.dumps(failure, ensure_ascii=False))

    def test_json_output_is_deterministic_and_has_no_absolute_source_path(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            path = Path(temp_dir) / "manifest.json"
            value = {"relativePath": "资料/sample.pdf", "tools": {"pdftotext": "v1"}}
            self.module.write_json(path, value)
            first = path.read_bytes()
            self.module.write_json(path, value)
            self.assertEqual(first, path.read_bytes())
            self.assertNotIn(str(Path(temp_dir)).encode(), first)

    def test_reuses_only_a_complete_source_with_verified_unit_hashes(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            root = Path(temp_dir)
            source, entry, manifest, _ = write_valid_pdf_output(root, self.module)
            reused = self.module.load_reusable_source(root, source, entry)
            self.assertIsNotNone(reused)
            (root / manifest["units"][0]["textFile"]).write_text("篡改", "utf-8")
            self.assertIsNone(self.module.load_reusable_source(root, source, entry))

    def test_reuse_rejects_missing_or_non_complete_error_report(self):
        for mutation in ("missing", "failed", "nonempty"):
            with self.subTest(mutation=mutation), tempfile.TemporaryDirectory() as temp_dir:
                root = Path(temp_dir)
                source, entry, _, _ = write_valid_pdf_output(root, self.module)
                if mutation == "missing":
                    (root / "errors.json").unlink()
                elif mutation == "failed":
                    self.module.write_json(root / "errors.json", {"status": "failed", "errors": []})
                else:
                    self.module.write_json(root / "errors.json", {"status": "complete", "errors": ["x"]})
                self.assertIsNone(self.module.load_reusable_source(root, source, entry))

    def test_reuse_rejects_missing_tampered_or_wrong_page_pdf_qa(self):
        for mutation in ("missing", "tampered", "wrong-page", "qa-checks"):
            with self.subTest(mutation=mutation), tempfile.TemporaryDirectory() as temp_dir:
                root = Path(temp_dir)
                source, entry, _, quality = write_valid_pdf_output(root, self.module)
                if mutation == "missing":
                    (root / "qa/cover.png").unlink()
                elif mutation == "tampered":
                    (root / "qa/selected-page.png").write_bytes(b"changed")
                else:
                    if mutation == "wrong-page":
                        quality["renders"][1]["page"] = 1
                    else:
                        checks = quality["renders"][1]["automatedChecks"]
                        checks["notBlank"] = not checks["notBlank"]
                    self.module.write_json(root / "quality.json", quality)
                self.assertIsNone(self.module.load_reusable_source(root, source, entry))

    def test_reuse_rejects_quality_or_manifest_contract_tampering(self):
        for mutation in ("quality", "manifest", "scope", "extra-field"):
            with self.subTest(mutation=mutation), tempfile.TemporaryDirectory() as temp_dir:
                root = Path(temp_dir)
                source, entry, manifest, quality = write_valid_pdf_output(root, self.module)
                if mutation == "quality":
                    quality["extractionQuality"]["characterCount"] += 1
                    self.module.write_json(root / "quality.json", quality)
                elif mutation == "manifest":
                    manifest["units"][0]["characterCount"] += 1
                    self.module.write_json(root / "manifest.json", manifest)
                elif mutation == "scope":
                    quality["scope"] = "被篡改的审核说明"
                    self.module.write_json(root / "quality.json", quality)
                else:
                    quality["unexpected"] = True
                    self.module.write_json(root / "quality.json", quality)
                self.assertIsNone(self.module.load_reusable_source(root, source, entry))

    def test_reuse_rejects_epub_or_office_quality_statistics_tampering(self):
        for unit_type, field in (("spine_item", "selectedSpineItems"),
                                 ("spine_item", "extractedParagraphs"),
                                 ("spine_item", "emptyTextUnits"),
                                 ("paragraph", "selectedParagraphs"),
                                 ("paragraph", "extractedParagraphs"),
                                 ("paragraph", "emptyTextUnits")):
            with self.subTest(unit_type=unit_type, field=field), tempfile.TemporaryDirectory() as temp_dir:
                root = Path(temp_dir)
                source, entry, _, quality = write_valid_non_pdf_output(root, self.module, unit_type)
                self.assertIsNotNone(self.module.load_reusable_source(root, source, entry))
                quality[field] += 1
                self.module.write_json(root / "quality.json", quality)
                self.assertIsNone(self.module.load_reusable_source(root, source, entry))

    def test_contact_sheets_are_built_from_verified_qa_and_hash_checked(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            root = Path(temp_dir)
            target = root / "sources/sample"
            source, entry, _, _ = write_valid_pdf_output(target, self.module)
            review = self.module.build_round_qa(root, [(source, entry, target, {1, 2})])
            self.module.validate_round_qa(root, review, [(source, entry, target, {1, 2})])
            self.assertEqual(2, review["renderCount"])
            self.assertEqual(1, len(review["contactSheets"]))
            sheet = root / review["contactSheets"][0]["file"]
            sheet.write_bytes(b"tampered")
            with self.assertRaisesRegex(ValueError, "联系表"):
                self.module.validate_round_qa(root, review, [(source, entry, target, {1, 2})])


if __name__ == "__main__":
    unittest.main()
