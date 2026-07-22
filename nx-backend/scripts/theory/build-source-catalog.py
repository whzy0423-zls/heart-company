#!/usr/bin/env python3
"""Build the complete Xinzhili source catalog and the bounded round-001 selection."""

import argparse
import hashlib
import html
import json
import math
import re
import subprocess
import zipfile
from collections import Counter, defaultdict
from pathlib import Path


CONTENT_EXTENSIONS = {".pdf", ".epub", ".doc", ".docx", ".pptx", ".mobi", ".azw3", ".jpg"}
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


def pdf_probe(path):
    info = run_text(["pdfinfo", str(path)])
    match = re.search(r"^Pages:\s+(\d+)$", info, re.MULTILINE)
    if not match:
        raise ValueError("pdfinfo 未返回页数")
    page_count = int(match.group(1))
    sample_pages = min(page_count, 5)
    sample = run_text(["pdftotext", "-f", "1", "-l", str(sample_pages), str(path), "-"])
    sample_chars = normalized_nonspace_length(sample)
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
    character_count = 0
    for _, raw in members:
        markup = raw.decode("utf-8", errors="ignore")
        markup = re.sub(r"<(script|style)\b.*?</\1>", " ", markup, flags=re.IGNORECASE | re.DOTALL)
        text = html.unescape(re.sub(r"<[^>]+>", " ", markup))
        character_count += normalized_nonspace_length(text)
    equivalent = max(1, math.ceil(character_count / chars_per_page))
    return {
        "extractionRoute": "epub_xhtml",
        "unitEstimate": {
            "unitType": "spine_item",
            "unitCount": len(members),
            "budgetPageEquivalent": equivalent,
            "normalizedTextCharacters": character_count,
        },
        "ocrPageEstimate": 0,
    }


def office_text_probe(path, chars_per_page):
    text = run_text(["textutil", "-convert", "txt", "-stdout", str(path)])
    character_count = normalized_nonspace_length(text)
    equivalent = max(1, math.ceil(character_count / chars_per_page))
    return {
        "extractionRoute": "textutil_plain_text",
        "unitEstimate": {
            "unitType": "normalized_text_page",
            "unitCount": equivalent,
            "budgetPageEquivalent": equivalent,
            "normalizedTextCharacters": character_count,
        },
        "ocrPageEstimate": 0,
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
    }


def probe_file(path, chars_per_page):
    suffix = path.suffix.lower()
    if suffix == ".pdf":
        return pdf_probe(path)
    if suffix == ".epub":
        return epub_probe(path, chars_per_page)
    if suffix in {".doc", ".docx"}:
        return office_text_probe(path, chars_per_page)
    if suffix == ".pptx":
        return pptx_probe(path)
    if suffix in {".mobi", ".azw3"}:
        return {
            "extractionRoute": "ebook_convert_required",
            "unitEstimate": {"unitType": "chapter", "unitCount": 0, "budgetPageEquivalent": 0},
            "ocrPageEstimate": 0,
        }
    if suffix == ".jpg":
        return {
            "extractionRoute": "image_ocr",
            "unitEstimate": {"unitType": "image", "unitCount": 1, "budgetPageEquivalent": 1},
            "ocrPageEstimate": 1,
        }
    raise ValueError(f"不支持的文件格式: {suffix}")


def stable_id(prefix, value):
    return f"{prefix}-{hashlib.sha256(value.encode('utf-8')).hexdigest()[:16]}"


def title_from_path(relative_path):
    title = Path(relative_path).stem
    title = re.sub(r"\(\d+\)$", "", title)
    return title.strip(" 《》_-")


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
        if source["processedUnitCount"] > estimate["unitCount"]:
            raise ValueError(f"选择范围超过实际单元数: {source['relativePath']}")
        if source["budgetPageEquivalent"] > estimate["budgetPageEquivalent"]:
            raise ValueError(f"折算预算超过来源总量: {source['relativePath']}")
        if source["ocrPageCount"] > source["processedUnitCount"]:
            raise ValueError(f"OCR 页数超过处理单元: {source['relativePath']}")
        if Path(source["relativePath"]).suffix.lower() in TEXT_EQUIVALENT_EXTENSIONS:
            if source["budgetPageEquivalent"] != estimate["budgetPageEquivalent"]:
                raise ValueError(f"文本折算预算与实际字符数不一致: {source['relativePath']}")
    return budget, ocr_pages


def build_catalog(source_root, selection):
    chars_per_page = selection["normalizedTextCharactersPerPage"]
    paths = sorted(
        (path for path in source_root.rglob("*") if path.is_file() and path.suffix.lower() in CONTENT_EXTENSIONS),
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
            probe = probe_file(path, chars_per_page)
        except (OSError, subprocess.CalledProcessError, ValueError, zipfile.BadZipFile) as exc:
            probe = {
                "extractionRoute": "manual_review_required",
                "unitEstimate": {"unitType": "unknown", "unitCount": 0, "budgetPageEquivalent": 0},
                "ocrPageEstimate": 0,
            }
            error = str(exc).splitlines()[0]

        if error:
            status, priority, batch, reason = "error", 9, "round-999", f"目录探测失败，需人工修复后重试：{error}"
        elif len(siblings) > 1 and relative_path != canonical_path:
            status, priority, batch = "duplicate", 9, "not-applicable"
            reason = f"SHA-256 与 {canonical_path} 相同，不重复计权。"
        elif relative_path in selected_paths:
            status, priority, batch, reason = "selected", 1, selection["roundId"], "进入首轮提炼范围。"
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
                "duplicateGroupId", "extractionRoute", "unitEstimate",
            )},
            **source,
        })

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
