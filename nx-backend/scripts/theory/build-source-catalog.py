#!/usr/bin/env python3
"""Build the complete Xinzhili source catalog and the bounded round-001 selection."""

import argparse
import hashlib
import html
import json
import math
import posixpath
import re
import subprocess
import sys
import zipfile
from collections import Counter, defaultdict
from pathlib import Path
from urllib.parse import unquote
from xml.etree import ElementTree


IGNORED_SYSTEM_FILES = {".DS_Store"}
TEXT_EQUIVALENT_EXTENSIONS = {".epub", ".doc", ".docx"}


def sha256_file(path):
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def normalized_nonspace_length(text):
    return len("".join(text.split()))


def run_text(command):
    result = subprocess.run(command, check=True, capture_output=True)
    return result.stdout.decode("utf-8", errors="ignore")


def command_version(command):
    result = subprocess.run(command, check=True, capture_output=True)
    output = (result.stdout + result.stderr).decode("utf-8", errors="ignore").strip()
    return output.splitlines()[0] if output else "version-unavailable"


def collect_tool_versions():
    macos_version = command_version(["sw_vers", "-productVersion"])
    return {
        "pdfinfo": command_version(["pdfinfo", "-v"]),
        "pdftotext": command_version(["pdftotext", "-v"]),
        "textutil": f"Apple textutil on macOS {macos_version}",
        "textutilBinarySha256": sha256_file(Path("/usr/bin/textutil")),
        "epubParser": f"python-{sys.version_info.major}.{sys.version_info.minor}.zipfile+ElementTree",
        "pptxParser": f"python-{sys.version_info.major}.{sys.version_info.minor}.zipfile+ElementTree",
        "catalogParser": "xinzhili-source-catalog-v1",
    }


def canonical_hash(value):
    payload = json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"))
    return hashlib.sha256(payload.encode("utf-8")).hexdigest()


def pdf_probe(path):
    info = run_text(["pdfinfo", str(path)])
    match = re.search(r"^Pages:\s+(\d+)$", info, re.MULTILINE)
    if not match:
        raise ValueError("pdfinfo 未返回页数")
    page_count = int(match.group(1))
    sample_pages = min(page_count, 5)
    sample = run_text(["pdftotext", "-f", "1", "-l", str(sample_pages), str(path), "-"])
    sample_chars = normalized_nonspace_length(sample)
    normalized_sample = "".join(sample.split())
    text_chars_per_page = sample_chars // max(sample_pages, 1)
    needs_ocr = text_chars_per_page < 80
    return {
        "extractionRoute": "pdf_ocr_selected" if needs_ocr else "pdf_text_layer",
        "unitEstimate": {
            "unitType": "page",
            "unitCount": page_count,
            "budgetPageEquivalent": page_count,
            "sampledPages": sample_pages,
            "sampleTextCharacters": sample_chars,
        },
        "ocrPageEstimate": page_count if needs_ocr else 0,
        "_normalizedProbeOutput": {
            "pageCount": page_count,
            "sampledPages": sample_pages,
            "normalizedSampleSha256": hashlib.sha256(normalized_sample.encode("utf-8")).hexdigest(),
            "normalizedSampleCharacters": sample_chars,
        },
    }


def safe_epub_members(path):
    with zipfile.ZipFile(path) as archive:
        members = []
        total_uncompressed = 0
        for item in archive.infolist():
            member_path = Path(item.filename)
            if member_path.is_absolute() or ".." in member_path.parts:
                raise ValueError("EPUB 包含不安全路径")
            total_uncompressed += item.file_size
            if total_uncompressed > 512 * 1024 * 1024:
                raise ValueError("EPUB 解压体积超过 512MiB")
            if item.filename.lower().endswith((".xhtml", ".html", ".htm")):
                members.append((item.filename, archive.read(item)))
        return members


def epub_probe(path, chars_per_page):
    members = safe_epub_members(path)
    member_map = {name: raw for name, raw in members}
    with zipfile.ZipFile(path) as archive:
        container = ElementTree.fromstring(archive.read("META-INF/container.xml"))
        rootfile = next(
            item.attrib["full-path"] for item in container.iter() if item.tag.endswith("rootfile")
        )
        package = ElementTree.fromstring(archive.read(rootfile))
        manifest = {
            item.attrib["id"]: item.attrib["href"]
            for item in package.iter()
            if item.tag.endswith("item") and "id" in item.attrib and "href" in item.attrib
        }
        base = posixpath.dirname(rootfile)
        spine = []
        for index, item in enumerate(
            (node for node in package.iter() if node.tag.endswith("itemref")), start=1
        ):
            idref = item.attrib["idref"]
            href = manifest.get(idref)
            if not href:
                raise ValueError(f"EPUB spine 引用缺失 manifest item: {idref}")
            member_name = posixpath.normpath(posixpath.join(base, unquote(href)))
            if member_name not in member_map:
                raise ValueError(f"EPUB spine 文件不存在: {member_name}")
            spine.append({"index": index, "idref": idref, "href": member_name})

    character_count = 0
    normalized_parts = []
    for item in spine:
        raw = member_map[item["href"]]
        markup = raw.decode("utf-8", errors="ignore")
        markup = re.sub(r"<(script|style)\b.*?</\1>", " ", markup, flags=re.IGNORECASE | re.DOTALL)
        text = html.unescape(re.sub(r"<[^>]+>", " ", markup))
        normalized = "".join(text.split())
        normalized_parts.append(normalized)
        character_count += len(normalized)
    equivalent = max(1, math.ceil(character_count / chars_per_page))
    return {
        "extractionRoute": "epub_xhtml",
        "unitEstimate": {
            "unitType": "spine_item",
            "unitCount": len(spine),
            "budgetPageEquivalent": equivalent,
            "normalizedTextCharacters": character_count,
            "locatorInventory": spine,
        },
        "ocrPageEstimate": 0,
        "_normalizedProbeOutput": {
            "spine": spine,
            "normalizedTextSha256": hashlib.sha256("".join(normalized_parts).encode("utf-8")).hexdigest(),
            "normalizedTextCharacters": character_count,
        },
    }


def office_text_probe(path, chars_per_page):
    text = run_text(["textutil", "-convert", "txt", "-stdout", str(path)])
    character_count = normalized_nonspace_length(text)
    equivalent = max(1, math.ceil(character_count / chars_per_page))
    paragraphs = [line.strip() for line in text.splitlines() if line.strip()]
    return {
        "extractionRoute": "textutil_plain_text",
        "unitEstimate": {
            "unitType": "paragraph",
            "unitCount": len(paragraphs),
            "budgetPageEquivalent": equivalent,
            "normalizedTextCharacters": character_count,
        },
        "ocrPageEstimate": 0,
        "_normalizedProbeOutput": {
            "paragraphCount": len(paragraphs),
            "normalizedTextSha256": hashlib.sha256("".join(text.split()).encode("utf-8")).hexdigest(),
            "normalizedTextCharacters": character_count,
        },
    }


def pptx_probe(path):
    with zipfile.ZipFile(path) as archive:
        slides = [
            item for item in archive.infolist()
            if re.fullmatch(r"ppt/slides/slide\d+\.xml", item.filename)
        ]
    count = max(1, len(slides))
    return {
        "extractionRoute": "pptx_slide_xml",
        "unitEstimate": {"unitType": "slide", "unitCount": count, "budgetPageEquivalent": count},
        "ocrPageEstimate": 0,
        "_normalizedProbeOutput": {"slideCount": count},
    }


def probe_file(path, chars_per_page, tool_versions):
    suffix = path.suffix.lower()
    if suffix == ".pdf":
        result = pdf_probe(path)
        tools = {key: tool_versions[key] for key in ("pdfinfo", "pdftotext")}
    elif suffix == ".epub":
        result = epub_probe(path, chars_per_page)
        tools = {"epubParser": tool_versions["epubParser"]}
    elif suffix in {".doc", ".docx"}:
        result = office_text_probe(path, chars_per_page)
        tools = {key: tool_versions[key] for key in ("textutil", "textutilBinarySha256")}
    elif suffix == ".pptx":
        result = pptx_probe(path)
        tools = {"pptxParser": tool_versions["pptxParser"]}
    elif suffix in {".mobi", ".azw3"}:
        result = {
            "extractionRoute": "ebook_convert_required",
            "unitEstimate": {"unitType": "chapter", "unitCount": 0, "budgetPageEquivalent": 0},
            "ocrPageEstimate": 0,
            "_normalizedProbeOutput": {"probeStatus": "converter-required", "byteSize": path.stat().st_size},
        }
        tools = {"catalogParser": tool_versions["catalogParser"]}
    elif suffix == ".jpg":
        result = {
            "extractionRoute": "image_ocr",
            "unitEstimate": {"unitType": "image", "unitCount": 1, "budgetPageEquivalent": 1},
            "ocrPageEstimate": 1,
            "_normalizedProbeOutput": {"imageCount": 1, "byteSize": path.stat().st_size},
        }
        tools = {"catalogParser": tool_versions["catalogParser"]}
    else:
        result = {
            "extractionRoute": "unsupported_format",
            "unitEstimate": {"unitType": "file", "unitCount": 1, "budgetPageEquivalent": 0},
            "ocrPageEstimate": 0,
            "_normalizedProbeOutput": {"probeStatus": "unsupported-format", "suffix": suffix},
        }
        tools = {"catalogParser": tool_versions["catalogParser"]}
    normalized_output = result.pop("_normalizedProbeOutput")
    result["probe"] = {
        "normalization": "xinzhili-probe-output-v1",
        "outputSha256": canonical_hash(normalized_output),
        "toolVersions": tools,
    }
    return result


def stable_id(prefix, value):
    return f"{prefix}-{hashlib.sha256(value.encode('utf-8')).hexdigest()[:16]}"


def title_from_path(relative_path):
    title = Path(relative_path).stem
    title = re.sub(r"\(\d+\)$", "", title)
    return title.strip(" 《》_-")


def range_contract(relative_path):
    suffix = Path(relative_path).suffix.lower()
    if suffix == ".pdf":
        return "page", "page"
    if suffix == ".epub":
        return "spine_item", "spine-item"
    if suffix in {".doc", ".docx"}:
        return "paragraph", "paragraph"
    if suffix == ".pptx":
        return "slide", "slide"
    raise ValueError(f"首轮来源格式不支持范围选择: {relative_path}")


def expand_selected_ranges(source, unit_count):
    expected_type, prefix = range_contract(source["relativePath"])
    if source["processedUnitType"] != expected_type:
        raise ValueError(f"单元类型与格式不匹配: {source['relativePath']}")
    selected_units = []
    pattern = re.compile(rf"^{re.escape(prefix)}:(\d+)(?:-(\d+))?$")
    for locator in source["selectedRanges"]:
        match = pattern.fullmatch(locator)
        if not match:
            raise ValueError(f"范围语法或类型非法: {source['relativePath']} {locator}")
        start = int(match.group(1))
        end = int(match.group(2) or start)
        if start < 1 or end < start:
            raise ValueError(f"范围起止非法: {source['relativePath']} {locator}")
        if end > unit_count:
            raise ValueError(f"选择范围越界: {source['relativePath']} {locator}")
        selected_units.extend(range(start, end + 1))
    if len(selected_units) != len(set(selected_units)):
        raise ValueError(f"选择范围重叠: {source['relativePath']}")
    if len(selected_units) != source["processedUnitCount"]:
        raise ValueError(
            f"选择范围展开数量与 processedUnitCount 不一致: {source['relativePath']}"
        )
    return selected_units, prefix


def collapse_ranges(unit_numbers, prefix):
    if not unit_numbers:
        return []
    ranges = []
    start = previous = unit_numbers[0]
    for number in unit_numbers[1:]:
        if number == previous + 1:
            previous = number
            continue
        ranges.append(f"{prefix}:{start}" if start == previous else f"{prefix}:{start}-{previous}")
        start = previous = number
    ranges.append(f"{prefix}:{start}" if start == previous else f"{prefix}:{start}-{previous}")
    return ranges


def validate_selection(selection, entries_by_path):
    sources = selection["sources"]
    if len(sources) > selection["maxSelectedFiles"]:
        raise ValueError("首轮选择文件数超过上限")
    if len({source["relativePath"] for source in sources}) != len(sources):
        raise ValueError("首轮选择包含重复路径")
    if any(source["relativePath"] not in entries_by_path for source in sources):
        missing = [source["relativePath"] for source in sources if source["relativePath"] not in entries_by_path]
        raise ValueError(f"首轮来源不存在: {missing}")

    hashes = [entries_by_path[source["relativePath"]]["sha256"] for source in sources]
    if len(hashes) != len(set(hashes)):
        raise ValueError("首轮选择包含 SHA-256 重复文件")
    budget = sum(source["budgetPageEquivalent"] for source in sources)
    ocr_pages = sum(source["ocrPageCount"] for source in sources)
    if budget > selection["maxBudgetPageEquivalent"]:
        raise ValueError("首轮处理页折算超过上限")
    if ocr_pages > selection["maxOcrPageCount"]:
        raise ValueError("首轮 OCR 页数超过上限")

    for source in sources:
        entry = entries_by_path[source["relativePath"]]
        estimate = entry["unitEstimate"]
        selected_units, _ = expand_selected_ranges(source, estimate["unitCount"])
        if source["budgetPageEquivalent"] > estimate["budgetPageEquivalent"]:
            raise ValueError(f"折算预算超过来源总量: {source['relativePath']}")
        if source["ocrPageCount"] > source["processedUnitCount"]:
            raise ValueError(f"OCR 页数超过处理单元: {source['relativePath']}")
        if Path(source["relativePath"]).suffix.lower() in TEXT_EQUIVALENT_EXTENSIONS:
            if source["budgetPageEquivalent"] != estimate["budgetPageEquivalent"]:
                raise ValueError(f"文本折算预算与实际字符数不一致: {source['relativePath']}")
        route = entry["extractionRoute"]
        if route == "pdf_ocr_selected":
            if not selected_units or source["ocrPageCount"] != len(selected_units):
                raise ValueError(f"OCR 页数必须与选定页数一致且大于零: {source['relativePath']}")
        elif source["ocrPageCount"] != 0:
            raise ValueError(f"非 OCR 抽取路线不得声明 OCR 页数: {source['relativePath']}")
    return budget, ocr_pages


def build_catalog(source_root, selection):
    chars_per_page = selection["normalizedTextCharactersPerPage"]
    tool_versions = collect_tool_versions()
    paths = sorted(
        (
            path for path in source_root.rglob("*")
            if path.is_file() and path.name not in IGNORED_SYSTEM_FILES
        ),
        key=lambda path: path.relative_to(source_root).as_posix(),
    )
    hashes = {path: sha256_file(path) for path in paths}
    hash_groups = defaultdict(list)
    for path, digest in hashes.items():
        hash_groups[digest].append(path)

    selected_paths = {source["relativePath"] for source in selection["sources"]}
    entries = []
    for path in paths:
        relative_path = path.relative_to(source_root).as_posix()
        digest = hashes[path]
        siblings = sorted(hash_groups[digest], key=lambda item: item.relative_to(source_root).as_posix())
        canonical_path = siblings[0].relative_to(source_root).as_posix()
        duplicate_group_id = stable_id("duplicate", digest) if len(siblings) > 1 else None
        canonical_work_id = stable_id("work", digest)
        error = None
        try:
            probe = probe_file(path, chars_per_page, tool_versions)
        except (
            KeyError,
            OSError,
            StopIteration,
            subprocess.CalledProcessError,
            ValueError,
            zipfile.BadZipFile,
            ElementTree.ParseError,
        ) as exc:
            normalized_error = {"errorType": type(exc).__name__, "suffix": path.suffix.lower()}
            probe = {
                "extractionRoute": "manual_review_required",
                "unitEstimate": {"unitType": "unknown", "unitCount": 0, "budgetPageEquivalent": 0},
                "ocrPageEstimate": 0,
                "probe": {
                    "normalization": "xinzhili-probe-output-v1",
                    "outputSha256": canonical_hash(normalized_error),
                    "toolVersions": {"catalogParser": tool_versions["catalogParser"]},
                },
            }
            error = str(exc).splitlines()[0]

        if error:
            status, priority, batch, reason = "error", 9, "round-999", f"目录探测失败，需人工修复后重试：{error}"
        elif len(siblings) > 1 and relative_path != canonical_path:
            status, priority, batch = "duplicate", 9, "not-applicable"
            reason = f"SHA-256 与 {canonical_path} 相同，不重复计权。"
        elif relative_path in selected_paths:
            status, priority, batch, reason = "selected", 1, selection["roundId"], "进入首轮提炼范围。"
        elif probe["extractionRoute"] == "unsupported_format":
            status, priority, batch = "excluded", 9, "not-applicable"
            reason = f"未识别格式 {path.suffix.lower() or '[无扩展名]'}；已纳入目录，需明确转换器后再处理。"
        elif path.suffix.lower() == ".jpg":
            status, priority, batch, reason = "excluded", 9, "not-applicable", "仅为封面图片，不作为独立理论来源。"
        else:
            status = "backlog"
            priority = 2 if probe["extractionRoute"] in {"pdf_text_layer", "epub_xhtml", "textutil_plain_text"} else 3
            batch = "round-002" if priority == 2 else "round-003"
            reason = "未进入首轮文件上限；已保留抽取路线，后续按批次处理。"

        entries.append({
            "fileId": stable_id("file", relative_path),
            "relativePath": relative_path,
            "sha256": digest,
            "byteSize": path.stat().st_size,
            "mediaType": path.suffix.lower().lstrip("."),
            "catalogStatus": status,
            "canonicalWorkId": canonical_work_id,
            "canonicalRelativePath": canonical_path,
            "duplicateGroupId": duplicate_group_id,
            **probe,
            "reason": reason,
            "priority": priority,
            "proposedBatch": batch,
        })

    entries_by_path = {entry["relativePath"]: entry for entry in entries}
    budget, ocr_pages = validate_selection(selection, entries_by_path)
    selected = []
    for source in selection["sources"]:
        catalog_entry = entries_by_path[source["relativePath"]]
        if catalog_entry["catalogStatus"] != "selected":
            raise ValueError(f"首轮来源不是 canonical selected 文件: {source['relativePath']}")
        selected.append({
            **{key: catalog_entry[key] for key in (
                "fileId", "relativePath", "sha256", "byteSize", "mediaType", "canonicalWorkId",
                "duplicateGroupId", "extractionRoute", "unitEstimate", "probe",
            )},
            **source,
        })

        selected_units, prefix = expand_selected_ranges(source, catalog_entry["unitEstimate"]["unitCount"])
        remaining_units = sorted(
            set(range(1, catalog_entry["unitEstimate"]["unitCount"] + 1)) - set(selected_units)
        )
        selected[-1]["remainingRanges"] = collapse_ranges(remaining_units, prefix)
        selected[-1]["remainingUnitCount"] = len(remaining_units)
        selected[-1]["proposedBatch"] = "round-002" if remaining_units else selection["roundId"]

    status_counts = Counter(entry["catalogStatus"] for entry in entries)
    for status in ("selected", "backlog", "duplicate", "excluded", "error"):
        status_counts.setdefault(status, 0)
    if sum(status_counts.values()) != len(paths):
        raise AssertionError("目录状态合计与实际内容文件数不一致")

    works = []
    selected_by_work = defaultdict(list)
    for item in selected:
        selected_by_work[item["canonicalWorkId"]].append(item)
    for work_id, files in sorted(selected_by_work.items()):
        works.append({
            "workId": work_id,
            "title": title_from_path(files[0]["relativePath"]),
            "sourceFileIds": [item["fileId"] for item in files],
            "selectionReason": files[0]["selectionReason"],
            "catalogStatus": "selected",
        })

    catalog = {
        "schemaVersion": "xinzhili.source-catalog.v1",
        "sourceCollection": source_root.name,
        "budgetRule": {
            "version": selection["budgetRuleVersion"],
            "normalizedTextCharactersPerPage": chars_per_page,
            "pdfAndPptx": "实际 page/slide",
            "epubDocDocx": "规范化非空 UTF-8 文本每 1800 字符向上取整",
        },
        "summary": {
            "physicalContentFileCount": len(paths),
            "statusCounts": dict(sorted(status_counts.items())),
            "duplicateGroupCount": sum(1 for group in hash_groups.values() if len(group) > 1),
        },
        "files": entries,
    }
    source_files = {
        "schemaVersion": "xinzhili.round-source-files.v1",
        "roundId": selection["roundId"],
        "summary": {
            "selectedCount": len(selected),
            "budgetPageEquivalent": budget,
            "ocrPageCount": ocr_pages,
            "limits": {
                "selectedFiles": selection["maxSelectedFiles"],
                "budgetPageEquivalent": selection["maxBudgetPageEquivalent"],
                "ocrPageCount": selection["maxOcrPageCount"],
            },
        },
        "files": selected,
    }
    works_document = {
        "schemaVersion": "xinzhili.round-works.v1",
        "roundId": selection["roundId"],
        "works": works,
    }
    return catalog, works_document, source_files


def write_json(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--source-root", type=Path, required=True)
    parser.add_argument("--selection", type=Path, required=True)
    parser.add_argument("--output-root", type=Path, required=True)
    args = parser.parse_args()

    selection = json.loads(args.selection.read_text(encoding="utf-8"))
    catalog, works, source_files = build_catalog(args.source_root, selection)
    write_json(args.output_root / "source-catalog.json", catalog)
    catalog_dir = args.output_root / selection["roundId"] / "catalog"
    write_json(catalog_dir / "works.json", works)
    write_json(catalog_dir / "source-files.json", source_files)
    print(json.dumps(source_files["summary"], ensure_ascii=False, sort_keys=True))


if __name__ == "__main__":
    main()
