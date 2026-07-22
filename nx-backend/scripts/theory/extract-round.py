#!/usr/bin/env python3
"""生成本地、可复现且可追溯的理论资料抽取工作包。"""

import argparse
import hashlib
import json
import os
import posixpath
import re
import shutil
import subprocess
import sys
import tempfile
import zipfile
from pathlib import Path, PurePosixPath
from urllib.parse import unquote

from lxml import etree


SCHEMA_VERSION = "xinzhili-extraction-work-package-v1"
MAX_EPUB_UNCOMPRESSED = 512 * 1024 * 1024
MAX_EPUB_MEMBER = 64 * 1024 * 1024
DEFAULT_COMPRESSION_RATIO = 200
PDF_SLICE_CHARACTERS = 4000
OCR_DPI = 150


def sha256_bytes(payload):
    return hashlib.sha256(payload).hexdigest()


def sha256_file(path):
    digest = hashlib.sha256()
    with Path(path).open("rb") as source:
        for block in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def write_json(path, value):
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    rendered = json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"))
    path.write_bytes((rendered + "\n").encode("utf-8"))


def safe_error(exc):
    message = str(exc).replace("\n", " ").replace("\r", " ")
    message = re.sub(r"(?:/[^\s:]+)+", "<path>", message)
    return {"errorType": type(exc).__name__, "reason": message[:500] or "未提供错误信息"}


def run_checked(command, *, text=False):
    try:
        result = subprocess.run(command, check=True, capture_output=True)
    except FileNotFoundError as exc:
        raise RuntimeError("缺少必需工具") from exc
    except subprocess.CalledProcessError as exc:
        raise RuntimeError(f"外部工具执行失败，退出码 {exc.returncode}") from exc
    return result.stdout.decode("utf-8", errors="strict") if text else result.stdout


def tool_version(command):
    result = subprocess.run(command, check=True, capture_output=True)
    output = (result.stdout + result.stderr).decode("utf-8", errors="replace").strip()
    return output.splitlines()[0] if output else "version-unavailable"


def require_runtime(*, needs_ocr, needs_office):
    required = ["pdfinfo", "pdftotext", "pdftoppm"]
    if needs_ocr:
        required.append("tesseract")
    if needs_office:
        required.append("textutil")
    missing = [name for name in required if shutil.which(name) is None]
    if missing:
        raise RuntimeError(f"缺少必需工具: {', '.join(sorted(missing))}")
    if needs_ocr:
        languages = set(run_checked(["tesseract", "--list-langs"], text=True).splitlines())
        missing_languages = {"chi_sim", "eng"} - languages
        if missing_languages:
            raise RuntimeError(f"缺少 Tesseract 语言包: {', '.join(sorted(missing_languages))}")
    versions = {
        "pdfinfo": tool_version(["pdfinfo", "-v"]),
        "pdftotext": tool_version(["pdftotext", "-v"]),
        "pdftoppm": tool_version(["pdftoppm", "-v"]),
        "epubParser": f"python-{sys.version_info.major}.{sys.version_info.minor}-lxml-{etree.LXML_VERSION}",
    }
    if needs_ocr:
        versions["tesseract"] = tool_version(["tesseract", "--version"])
        versions["tesseractLanguages"] = ["chi_sim", "eng"]
    if needs_office:
        versions["textutil"] = "Apple textutil"
        versions["textutilBinarySha256"] = sha256_file(Path("/usr/bin/textutil"))
    return versions


def parse_selected_ranges(ranges, expected_prefix):
    selected = set()
    for value in ranges:
        match = re.fullmatch(r"([a-z-]+):(\d+)(?:-(\d+))?", value)
        if not match or match.group(1) != expected_prefix:
            raise ValueError(f"selectedRanges 格式不匹配: {value}")
        start, end = int(match.group(2)), int(match.group(3) or match.group(2))
        if start < 1 or end < start:
            raise ValueError(f"selectedRanges 范围无效: {value}")
        new_values = set(range(start, end + 1))
        if selected.intersection(new_values):
            raise ValueError(f"selectedRanges 存在重叠: {value}")
        selected.update(new_values)
    return selected


def selection_totals(selection):
    sources = selection.get("sources", [])
    return {
        "sourceCount": len(sources),
        "processedUnitCount": sum(item["processedUnitCount"] for item in sources),
        "budgetPageEquivalent": sum(item["budgetPageEquivalent"] for item in sources),
        "ocrPageCount": sum(item["ocrPageCount"] for item in sources),
    }


def validate_inputs(selection, catalog, source_root):
    totals = selection_totals(selection)
    if totals["sourceCount"] > selection["maxSelectedFiles"]:
        raise ValueError("选中来源数量超过上限")
    if totals["budgetPageEquivalent"] > selection["maxBudgetPageEquivalent"]:
        raise ValueError("总处理预算超过上限")
    if totals["ocrPageCount"] > selection["maxOcrPageCount"]:
        raise ValueError("OCR 页预算超过上限")
    catalog_by_path = {entry["relativePath"]: entry for entry in catalog.get("files", [])}
    root = Path(source_root).resolve()
    validated = []
    prefixes = {"page": "page", "spine_item": "spine-item", "paragraph": "paragraph"}
    for source in selection["sources"]:
        relative_path = source["relativePath"]
        relative = Path(relative_path)
        if relative.is_absolute() or ".." in relative.parts:
            raise ValueError("来源路径不安全")
        entry = catalog_by_path.get(relative_path)
        if not entry or entry.get("catalogStatus") != "selected":
            raise ValueError(f"目录中不存在 selected 来源: {relative_path}")
        unit_type = source["processedUnitType"]
        if unit_type not in prefixes:
            raise ValueError("不支持的处理单元类型")
        selected = parse_selected_ranges(source["selectedRanges"], prefixes[unit_type])
        if len(selected) != source["processedUnitCount"]:
            raise ValueError(f"selectedRanges 与 processedUnitCount 不一致: {relative_path}")
        if max(selected) > entry["unitEstimate"]["unitCount"]:
            raise ValueError(f"selectedRanges 超出目录单元数: {relative_path}")
        if unit_type == "page" and source["budgetPageEquivalent"] != len(selected):
            raise ValueError(f"页预算与 selectedRanges 不一致: {relative_path}")
        if source["ocrPageCount"] > len(selected):
            raise ValueError(f"OCR 页数超过 selectedRanges: {relative_path}")
        path = (root / relative).resolve()
        try:
            path.relative_to(root)
        except ValueError as exc:
            raise ValueError("来源路径越界") from exc
        if not path.is_file():
            raise ValueError(f"来源文件不存在: {relative_path}")
        if sha256_file(path) != entry["sha256"]:
            raise ValueError(f"来源 SHA-256 与目录不一致: {relative_path}")
        if entry["unitEstimate"]["unitType"] != unit_type:
            raise ValueError(f"来源单元类型与目录不一致: {relative_path}")
        validated.append((source, entry, path, selected))
    return validated


def slice_pdf_page(page, text, limit=PDF_SLICE_CHARACTERS):
    if limit < 1:
        raise ValueError("切片字符上限必须为正整数")
    parts = [""] if not text else [text[index : index + limit] for index in range(0, len(text), limit)]
    records, offset = [], 0
    for index, part in enumerate(parts, start=1):
        records.append({"locator": {"page": page, "slice": index, "characterStart": offset,
                                     "characterEnd": offset + len(part)}, "text": part})
        offset += len(part)
    return records


def is_heading(text):
    value = text.strip()
    return bool(re.match(r"^第.{1,12}[章节篇部卷]", value)
                or re.match(r"^[一二三四五六七八九十百]+[、.．]", value)
                or re.match(r"^\d+[、.．]\s*\S+", value))


def office_units(text, selected):
    paragraphs = [line.strip() for line in text.splitlines() if line.strip()]
    units, heading = [], None
    for index, paragraph in enumerate(paragraphs, start=1):
        if is_heading(paragraph):
            heading = paragraph
        if index in selected:
            units.append({"locator": {"heading": heading, "paragraph": index},
                          "text": paragraph, "confidence": 1.0})
    if {item["locator"]["paragraph"] for item in units} != selected:
        raise ValueError("textutil 段落数量少于 selectedRanges")
    return units


def _safe_epub_name(name):
    decoded = unquote(name).replace("\\", "/")
    path = PurePosixPath(decoded)
    if path.is_absolute() or ".." in path.parts or not decoded or chr(0) in decoded:
        raise ValueError("EPUB 包含不安全路径")
    return posixpath.normpath(decoded)


def _read_xml(raw, label):
    parser = etree.XMLParser(resolve_entities=False, no_network=True, recover=False, huge_tree=False)
    try:
        return etree.fromstring(raw, parser=parser)
    except (etree.XMLSyntaxError, ValueError) as exc:
        raise ValueError(f"EPUB {label} XML/XHTML 格式错误") from exc


def read_epub_spine(path, *, max_compression_ratio=DEFAULT_COMPRESSION_RATIO):
    try:
        archive = zipfile.ZipFile(path)
    except (zipfile.BadZipFile, OSError) as exc:
        raise ValueError("EPUB 不是有效 ZIP") from exc
    with archive:
        members, total = {}, 0
        for info in archive.infolist():
            name = _safe_epub_name(info.filename)
            total += info.file_size
            if info.file_size > MAX_EPUB_MEMBER or total > MAX_EPUB_UNCOMPRESSED:
                raise ValueError("EPUB 解压体积超过上限")
            if info.file_size and (not info.compress_size or info.file_size / info.compress_size > max_compression_ratio):
                raise ValueError("EPUB 压缩比超过上限")
            members[name] = info
        if "META-INF/container.xml" not in members:
            raise ValueError("EPUB 缺少 container.xml")
        container = _read_xml(archive.read(members["META-INF/container.xml"]), "container")
        rootfiles = container.xpath('//*[local-name()="rootfile"]/@full-path')
        if len(rootfiles) != 1:
            raise ValueError("EPUB rootfile 数量无效")
        rootfile = _safe_epub_name(rootfiles[0])
        if rootfile not in members:
            raise ValueError("EPUB package 文件不存在")
        package = _read_xml(archive.read(members[rootfile]), "package")
        manifest = {item.get("id"): item.get("href") for item in
                    package.xpath('//*[local-name()="manifest"]/*[local-name()="item"]')
                    if item.get("id") and item.get("href")}
        base, spine = posixpath.dirname(rootfile), []
        for index, itemref in enumerate(package.xpath(
                '//*[local-name()="spine"]/*[local-name()="itemref"]'), start=1):
            idref = itemref.get("idref")
            if idref not in manifest:
                raise ValueError("EPUB spine 引用缺少 manifest item")
            member = _safe_epub_name(posixpath.join(base, unquote(manifest[idref]).split("#", 1)[0]))
            if member not in members:
                raise ValueError("EPUB spine 文件不存在")
            spine.append({"index": index, "raw": archive.read(members[member])})
        if not spine:
            raise ValueError("EPUB spine 为空")
        return spine


def epub_units(path, selected):
    spine = read_epub_spine(path)
    if max(selected) > len(spine):
        raise ValueError("EPUB selectedRanges 超出 spine")
    units = []
    for item in spine:
        if item["index"] not in selected:
            continue
        document = _read_xml(item["raw"], "XHTML")
        headings = document.xpath('//*[local-name()="h1" or local-name()="h2" or local-name()="h3" '
                                  'or local-name()="h4" or local-name()="h5" or local-name()="h6"]')
        chapter = " ".join("".join(headings[0].itertext()).split()) if headings else ""
        if not chapter:
            titles = document.xpath('//*[local-name()="title"]')
            chapter = " ".join("".join(titles[0].itertext()).split()) if titles else ""
        chapter = chapter or f"spine-{item['index']:04d}"
        paragraph_number = 0
        block_tags = {"p", "li", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote", "figcaption"}
        semantic_nodes = []
        for node in document.iter():
            tag = etree.QName(node).localname if isinstance(node.tag, str) else ""
            descendant_tags = {etree.QName(child).localname for child in node.iterdescendants()
                               if isinstance(child.tag, str)}
            if tag in block_tags:
                semantic_nodes.append(node)
            elif tag == "div" and not descendant_tags.intersection(block_tags | {"div"}):
                semantic_nodes.append(node)
            elif tag == "a":
                ancestor_tags = {etree.QName(parent).localname for parent in node.iterancestors()
                                 if isinstance(parent.tag, str)}
                if not ancestor_tags.intersection(block_tags | {"div"}):
                    semantic_nodes.append(node)
        for node in semantic_nodes:
            text = " ".join("".join(node.itertext()).split())
            if text:
                paragraph_number += 1
                units.append({"locator": {"spineItem": item["index"], "chapter": chapter,
                                            "paragraph": paragraph_number},
                              "text": text, "confidence": 1.0, "emptyText": False})
        if not paragraph_number:
            image_alts = [" ".join(value.split()) for value in
                          document.xpath('//*[local-name()="img"]/@alt') if value.strip()]
            has_cover_image = bool(document.xpath('//*[local-name()="img" or local-name()="image"]'))
            if not image_alts and has_cover_image:
                image_alts = [] if chapter.startswith("spine-") else [chapter]
            fallback = "；".join(image_alts)
            units.append({"locator": {"spineItem": item["index"], "chapter": chapter, "paragraph": 1},
                          "text": fallback, "confidence": 1.0, "emptyText": not bool(fallback)})
    return units


def write_text_unit(root, relative_file, text, locator, confidence, empty_text=False):
    payload = text.encode("utf-8", errors="strict")
    path = Path(root) / relative_file
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(payload)
    return {"locator": locator, "textFile": relative_file, "encoding": "utf-8",
            "textSha256": sha256_bytes(payload), "utf8Bytes": len(payload),
            "characterCount": len(text), "nonWhitespaceCharacterCount": len("".join(text.split())),
            "confidence": round(float(confidence), 6), "emptyText": bool(empty_text)}


def validate_source_manifest(manifest, unit_type):
    required = {"status", "sourceSha256", "processedUnitCount", "budgetPageEquivalent",
                "ocrPageCount", "units"}
    if required - set(manifest):
        raise ValueError("manifest 缺少字段")
    if manifest["status"] != "complete" or not re.fullmatch(r"[0-9a-f]{64}", manifest["sourceSha256"]):
        raise ValueError("manifest 状态或 source SHA-256 无效")
    fields = {"page": {"page", "slice", "characterStart", "characterEnd"},
              "spine_item": {"spineItem", "chapter", "paragraph"},
              "paragraph": {"heading", "paragraph"}}[unit_type]
    for unit in manifest["units"]:
        if not fields.issubset(unit.get("locator", {})):
            raise ValueError("manifest unit locator 不可回溯")
        if "characterCount" not in unit or "confidence" not in unit:
            raise ValueError("manifest unit 缺少质量字段")


def atomic_source_output(output, producer):
    output, staging = Path(output), Path(str(output) + ".staging")
    shutil.rmtree(staging, ignore_errors=True)
    staging.mkdir(parents=True, exist_ok=True)
    try:
        result = producer(staging)
        if not (staging / "manifest.json").is_file():
            raise RuntimeError("抽取器未生成 manifest.json")
        shutil.rmtree(output, ignore_errors=True)
        staging.replace(output)
        return result
    except Exception as exc:
        shutil.rmtree(staging, ignore_errors=True)
        shutil.rmtree(output, ignore_errors=True)
        output.mkdir(parents=True, exist_ok=True)
        write_json(output / "errors.json", {"status": "failed", **safe_error(exc)})
        raise


def extract_pdf_text(path, page):
    text = run_checked(["pdftotext", "-f", str(page), "-l", str(page), "-enc", "UTF-8",
                        "-layout", str(path), "-"], text=True)
    return text.replace("\f", "").strip()


def render_pdf_page(path, page, prefix, dpi):
    run_checked(["pdftoppm", "-f", str(page), "-l", str(page), "-r", str(dpi), "-png",
                 "-singlefile", str(path), str(prefix)])
    rendered = Path(str(prefix) + ".png")
    if not rendered.is_file() or not rendered.stat().st_size:
        raise RuntimeError("PDF 页面渲染未生成图像")
    return rendered


def ocr_pdf_page(path, page, temp_dir):
    image = render_pdf_page(path, page, Path(temp_dir) / f"ocr-{page:04d}", OCR_DPI)
    output = Path(temp_dir) / f"tesseract-{page:04d}"
    run_checked(["tesseract", str(image), str(output), "-l", "chi_sim+eng", "--psm", "6", "txt", "tsv"])
    text = output.with_suffix(".txt").read_text("utf-8", errors="strict").strip()
    confidences = []
    for row in output.with_suffix(".tsv").read_text("utf-8", errors="strict").splitlines()[1:]:
        columns = row.split("\t")
        if len(columns) >= 12 and columns[11].strip():
            try:
                confidence = float(columns[10])
            except ValueError:
                continue
            if confidence >= 0:
                confidences.append(confidence / 100)
    return text, (sum(confidences) / len(confidences) if confidences else 0.0)


def inspect_png(path):
    try:
        from PIL import Image, ImageStat
        with Image.open(path) as image:
            gray, pixels = image.convert("L"), max(1, image.width * image.height)
            histogram = gray.histogram()
            dark, light = sum(histogram[:8]) / pixels, sum(histogram[248:]) / pixels
            bbox = gray.point(lambda value: 255 if value < 245 else 0).getbbox()
            edge = bool(bbox and (bbox[0] <= 1 or bbox[1] <= 1 or bbox[2] >= image.width - 1
                                  or bbox[3] >= image.height - 1))
            return {"width": image.width, "height": image.height,
                    "meanLuminance": round(ImageStat.Stat(gray).mean[0], 3),
                    "darkPixelFraction": round(dark, 6), "lightPixelFraction": round(light, 6),
                    "notBlack": dark < .98, "notBlank": light < .998,
                    "possibleEdgeCropping": edge, "humanReviewStatus": "pending"}
    except ImportError:
        return {"automatedInspection": "Pillow unavailable", "humanReviewStatus": "pending"}


def create_pdf_qa(path, staging, selected):
    qa_dir = staging / "qa"
    qa_dir.mkdir(parents=True, exist_ok=True)
    records = []
    for label, page in (("cover", 1), ("selected-page", max(selected))):
        image = render_pdf_page(path, page, qa_dir / label, 120)
        records.append({"kind": label, "page": page, "imageFile": image.relative_to(staging).as_posix(),
                        "imageSha256": sha256_file(image), "automatedChecks": inspect_png(image)})
    quality = {"status": "pending_human_review",
               "scope": "封面及一个选中页面；自动检查不等于人工通过", "renders": records}
    write_json(staging / "quality.json", quality)
    return quality


def unit_filename(unit_type, locator):
    if unit_type == "page":
        return f"units/page-{locator['page']:04d}-slice-{locator['slice']:02d}.txt"
    if unit_type == "spine_item":
        return f"units/spine-{locator['spineItem']:04d}-paragraph-{locator['paragraph']:05d}.txt"
    return f"units/paragraph-{locator['paragraph']:05d}.txt"


def distinct_unit_count(records, unit_type):
    locator_field = {"page": "page", "spine_item": "spineItem", "paragraph": "paragraph"}[unit_type]
    return len({record["locator"][locator_field] for record in records})


def summarize_extraction_quality(records):
    confidences = [record["confidence"] for record in records]
    return {
        "unitFileCount": len(records),
        "emptyTextUnitCount": sum(1 for record in records if record["characterCount"] == 0),
        "minimumConfidence": min(confidences),
        "averageConfidence": round(sum(confidences) / len(confidences), 6),
        "characterCount": sum(record["characterCount"] for record in records),
    }


def extract_source(source, entry, path, selected, output, tools):
    unit_type = source["processedUnitType"]
    def produce(staging):
        route, extracted = entry["extractionRoute"], []
        if unit_type == "page":
            use_ocr = source["ocrPageCount"] > 0
            if use_ocr and source["ocrPageCount"] != len(selected):
                raise ValueError("OCR 页未精确覆盖所有选中页")
            with tempfile.TemporaryDirectory(dir=staging) as temp_dir:
                for page in sorted(selected):
                    text, confidence = (ocr_pdf_page(path, page, temp_dir) if use_ocr
                                        else (extract_pdf_text(path, page), 1.0))
                    for item in slice_pdf_page(page, text):
                        item["confidence"] = confidence
                        extracted.append(item)
            quality = create_pdf_qa(path, staging, selected)
        elif unit_type == "spine_item":
            extracted = epub_units(path, selected)
            quality = {"status": "pending_human_review", "selectedSpineItems": len(selected),
                       "emptyTextUnits": sum(1 for item in extracted if item.get("emptyText")),
                       "scope": "严格 XHTML 语义块与空单元覆盖检查；不等于人工通过"}
            write_json(staging / "quality.json", quality)
        else:
            text = run_checked(["textutil", "-convert", "txt", "-stdout", str(path)], text=True)
            extracted = office_units(text, selected)
            quality = {"status": "pending_human_review", "selectedParagraphs": len(selected),
                       "scope": "textutil UTF-8 转换与段落覆盖检查；不等于人工通过"}
            write_json(staging / "quality.json", quality)
        records = [write_text_unit(staging, unit_filename(unit_type, item["locator"]), item["text"],
                                   item["locator"], item.get("confidence", 1.0),
                                   item.get("emptyText", False)) for item in extracted]
        if not records:
            raise ValueError("来源未抽取到任何 UTF-8 文本")
        distinct = distinct_unit_count(records, unit_type)
        if distinct != source["processedUnitCount"]:
            raise ValueError("实际处理单元数与选择合同不一致")
        quality["extractionQuality"] = summarize_extraction_quality(records)
        write_json(staging / "quality.json", quality)
        parameters = {
            "pdf_text_layer": ["pdftotext -f <PAGE> -l <PAGE> -enc UTF-8 -layout <SOURCE> -"],
            "pdf_ocr_selected": [f"pdftoppm -r {OCR_DPI} -png -singlefile <SOURCE> <TEMP>",
                                 "tesseract <IMAGE> <OUTPUT> -l chi_sim+eng --psm 6 txt tsv"],
            "epub_xhtml": ["strict ZIP + OPF spine + lxml XMLParser(no_network,no_entities)"],
            "textutil_plain_text": ["textutil -convert txt -stdout <SOURCE>"],
        }[route]
        manifest = {"schemaVersion": SCHEMA_VERSION, "status": "complete", "roundId": "round-001",
                    "relativePath": source["relativePath"], "sourceSha256": entry["sha256"],
                    "extractionRoute": route, "selectedRanges": source["selectedRanges"],
                    "processedUnitType": unit_type, "processedUnitCount": distinct,
                    "budgetPageEquivalent": source["budgetPageEquivalent"],
                    "ocrPageCount": source["ocrPageCount"], "tools": tools, "parameters": parameters,
                    "units": records, "qualityReport": "quality.json", "errorReport": "errors.json"}
        if unit_type == "spine_item":
            manifest["semanticBlockContract"] = "v1"
        validate_source_manifest(manifest, unit_type)
        write_json(staging / "errors.json", {"status": "complete", "errors": []})
        write_json(staging / "manifest.json", manifest)
        return manifest, quality
    return atomic_source_output(output, produce)


def source_directory_name(relative_path, source_sha):
    slug = re.sub(r"[^a-zA-Z0-9]+", "-", Path(relative_path).stem).strip("-").lower()[:32]
    return f"{source_sha[:16]}-{slug or 'source'}"


def load_reusable_source(target, source, entry):
    target = Path(target)
    try:
        manifest = json.loads((target / "manifest.json").read_text("utf-8"))
        quality = json.loads((target / "quality.json").read_text("utf-8"))
    except (OSError, UnicodeError, json.JSONDecodeError):
        return None
    expected = {
        "status": "complete",
        "sourceSha256": entry["sha256"],
        "selectedRanges": source["selectedRanges"],
        "processedUnitType": source["processedUnitType"],
        "processedUnitCount": source["processedUnitCount"],
        "budgetPageEquivalent": source["budgetPageEquivalent"],
        "ocrPageCount": source["ocrPageCount"],
    }
    if any(manifest.get(key) != value for key, value in expected.items()):
        return None
    if source["processedUnitType"] == "spine_item" and manifest.get("semanticBlockContract") != "v1":
        return None
    for unit in manifest.get("units", []):
        text_file = unit.get("textFile", "")
        relative = Path(text_file)
        if relative.is_absolute() or ".." in relative.parts:
            return None
        path = target / relative
        if not path.is_file() or sha256_file(path) != unit.get("textSha256"):
            return None
    if not manifest.get("units") or quality.get("status") != "pending_human_review":
        return None
    return manifest, quality


def build_round(selection_path, catalog_path, source_root, output_root):
    selection = json.loads(Path(selection_path).read_text("utf-8"))
    catalog = json.loads(Path(catalog_path).read_text("utf-8"))
    validated = validate_inputs(selection, catalog, source_root)
    tools = require_runtime(needs_ocr=any(s["ocrPageCount"] for s, _, _, _ in validated),
                            needs_office=any(s["processedUnitType"] == "paragraph"
                                             for s, _, _, _ in validated))
    output_root = Path(output_root)
    output_root.mkdir(parents=True, exist_ok=True)
    (output_root / "round-manifest.json").unlink(missing_ok=True)
    summaries, failures = [], []
    for source, entry, path, selected in validated:
        directory = source_directory_name(source["relativePath"], entry["sha256"])
        target = output_root / "sources" / directory
        try:
            reusable = load_reusable_source(target, source, entry)
            manifest, quality = (reusable if reusable is not None else
                                 extract_source(source, entry, path, selected, target, tools))
            if "extractionQuality" not in quality:
                quality["extractionQuality"] = summarize_extraction_quality(manifest["units"])
                write_json(target / "quality.json", quality)
            summaries.append({"relativePath": source["relativePath"],
                              "directory": f"sources/{directory}",
                              "manifestSha256": sha256_file(target / "manifest.json"),
                              "unitFileCount": len(manifest["units"]),
                              "qualityStatus": quality["status"]})
        except Exception as exc:
            failures.append({"relativePath": source["relativePath"], **safe_error(exc)})
            break
    if failures:
        write_json(output_root / "round-errors.json", {"status": "failed", "failures": failures})
        raise RuntimeError(f"首轮抽取失败: {failures[0]['relativePath']}")
    totals = selection_totals(selection)
    manifest = {"schemaVersion": SCHEMA_VERSION, "status": "complete", "roundId": selection["roundId"],
                "selectionSha256": sha256_file(selection_path), "catalogSha256": sha256_file(catalog_path),
                "budgetRuleVersion": selection["budgetRuleVersion"],
                "normalizedTextCharactersPerPage": selection["normalizedTextCharactersPerPage"],
                "totals": totals,
                "limits": {"maxSelectedFiles": selection["maxSelectedFiles"],
                           "maxBudgetPageEquivalent": selection["maxBudgetPageEquivalent"],
                           "maxOcrPageCount": selection["maxOcrPageCount"]},
                "sources": summaries, "humanReviewStatus": "pending"}
    write_json(output_root / "round-errors.json", {"status": "complete", "errors": []})
    write_json(output_root / "round-manifest.json", manifest)
    return manifest


def parse_args(argv=None):
    repository_root = Path(__file__).resolve().parents[3]
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--selection", type=Path,
                        default=Path(__file__).resolve().parent / "round-001-selection.json")
    parser.add_argument("--catalog", type=Path,
                        default=repository_root / "data/theory/xinzhili/source-catalog.json")
    parser.add_argument("--source-root", type=Path, default=os.environ.get("THEORY_SOURCE_ROOT"))
    parser.add_argument("--output", type=Path,
                        default=repository_root / "var/theory-work/xinzhili/round-001")
    args = parser.parse_args(argv)
    if args.source_root is None:
        parser.error("必须通过 --source-root 或 THEORY_SOURCE_ROOT 显式提供原始资料目录")
    return args


def main(argv=None):
    args = parse_args(argv)
    try:
        manifest = build_round(args.selection, args.catalog, args.source_root, args.output)
    except Exception as exc:
        print(f"抽取失败: {safe_error(exc)['reason']}", file=sys.stderr)
        return 1
    totals = manifest["totals"]
    print(f"抽取完成：{totals['sourceCount']} 个来源，{totals['budgetPageEquivalent']} 页等价，"
          f"{totals['ocrPageCount']} 页 OCR")
    print(f"本地工作包：{args.output}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
