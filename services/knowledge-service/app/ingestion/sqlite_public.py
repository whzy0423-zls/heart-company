"""Read-only SQLite catalog and bounded public-knowledge import commands."""

from __future__ import annotations

import argparse
from collections import Counter
from collections.abc import Iterator
from contextlib import closing, contextmanager
import hashlib
import json
import os
from pathlib import Path
import re
import sqlite3
import tempfile
import unicodedata
from typing import Any

import psycopg
from psycopg.types.json import Jsonb
from app.retrieval.tokenization import search_tokens


CLEANING_VERSION = "sqlite-clean-v1"
MAX_INPUT_CHARS = 100_000
_GROWTH = re.compile("心理|人格|沟通|认知|学习|教育|情绪|自我成长|咨询|精神健康", re.I)
_AD = re.compile("扫码.{0,12}(微信|购买|下载)|加微信.{0,15}(购买|完整版)|电子书.{0,10}(下载|购买)")
_COMMERCIAL_SIGNALS = ("欢迎光临本店", "淘宝第一图书馆", "发货QQ", "资料终身质保", "敬告倒卖")
_PAGE_MARKER = re.compile(r"^(?:第\s*\d+\s*页|[-—]?\s*\d{1,5}\s*[-—]?)$")
_NAMESPACE = re.compile(r"[A-Za-z0-9][A-Za-z0-9_.-]{0,79}\Z")
_NORMAL_EXTRACTION_STATUSES = ("ready", "ok", "success", "completed", "done", "legacy", "extracted",
                               "imported", "indexed", "pending", "ocr_running", "running", "processing")


def _dataset_id(value: str) -> str:
    if not _NAMESPACE.fullmatch(value):
        raise ValueError("dataset-id must be a stable ASCII name (letters, digits, dot, dash, underscore)")
    return value


@contextmanager
def open_readonly(path: str | Path) -> Iterator[sqlite3.Connection]:
    db = sqlite3.connect(Path(path).resolve().as_uri() + "?mode=ro", uri=True)
    db.row_factory = sqlite3.Row
    db.execute("PRAGMA query_only=ON")
    try:
        yield db
    finally:
        db.close()


def _quality(status: str) -> str:
    if status.lower() in _NORMAL_EXTRACTION_STATUSES:
        return "pending"
    return "needs_review"


def read_catalog(path: str | Path, dataset_id: str) -> Iterator[dict[str, Any]]:
    dataset_id = _dataset_id(dataset_id)
    with open_readonly(path) as db:
        categories = {row["id"]: row["name"] for row in db.execute("SELECT id,name FROM library_categories")}
        secondary: dict[int, list[dict[str, Any]]] = {}
        for row in db.execute("SELECT book_id,category_id,is_primary,confidence,reason FROM book_categories ORDER BY book_id,category_id"):
            secondary.setdefault(row["book_id"], []).append({
                "name": categories.get(row["category_id"], "未分类"),
                "is_primary": bool(row["is_primary"]), "confidence": row["confidence"], "reason": row["reason"],
            })
        story_categories = {row["category_key"]: row["label"] for row in db.execute("SELECT category_key,label FROM story_library_categories")}
        for kind, table in (("book", "books"), ("story", "story_library_sources")):
            for result in db.execute(f"SELECT * FROM {table} ORDER BY id"):
                row = dict(result)
                category = categories.get(row.get("category_id"), "未分类") if kind == "book" else story_categories.get(row.get("category_key"), row.get("category_key", "未分类"))
                status = str(row.get("extract_status") or "legacy")
                source_categories = secondary.get(row["id"], []) if kind == "book" else []
                if not source_categories:
                    source_categories = [{"name": category, "is_primary": True, "confidence": 1, "reason": ""}]
                # Preserve original ingestion metadata without scanning any body text.
                metadata = {key: value for key, value in row.items() if key not in {"id", "title"}}
                metadata.update({"categories": source_categories, "original_source_id": row["id"]})
                yield {
                    "id": f"{dataset_id}:{kind}:{row['id']}", "dataset_id": dataset_id,
                    "source_kind": kind, "source_record_id": row["id"], "title": row["title"],
                    "category": category, "file_format": row.get("file_format") or "",
                    "extract_status": status, "source_chunks": row.get("total_chunks" if kind == "book" else "chunk_count") or 0,
                    "text_chars": row.get("text_chars" if kind == "book" else "char_count") or 0,
                    "enabled": kind == "book" and bool(_GROWTH.search(category)),
                    "quality_status": _quality(status), "metadata": metadata,
                }


def clean_text(text: str) -> tuple[str, str | None, list[str]]:
    if len(text) > MAX_INPUT_CHARS:
        return "", "oversized", []
    normalized = unicodedata.normalize("NFC", text.replace("\r\n", "\n").replace("\r", "\n"))
    normalized = "".join(char for char in normalized if char in "\n\t" or unicodedata.category(char) != "Cc")
    normalized = "\n".join(re.sub(r"[ \t]+", " ", line).strip() for line in normalized.splitlines())
    normalized = re.sub(r"\n{3,}", "\n\n", normalized).strip()
    if not normalized:
        return "", "empty", []
    useful = sum(char.isalnum() for char in normalized)
    if useful < 4 or normalized.count("\ufffd") / len(normalized) > 0.15 or useful / len(normalized) < 0.25:
        return "", "garbage", []
    # Only reject ad-only short fragments; retain mixed prose and uncertain OCR for review.
    commercial_signals = sum(signal in normalized for signal in _COMMERCIAL_SIGNALS)
    if (len(normalized) < 180 and _AD.search(normalized)) or (len(normalized) < 1000 and commercial_signals >= 2):
        return "", "advertisement", []
    flags = ["ocr_uncertainty"] if "\ufffd" in normalized else []
    lines = normalized.splitlines()
    if len(lines) > 1 and (_PAGE_MARKER.fullmatch(lines[0]) or _PAGE_MARKER.fullmatch(lines[-1])):
        flags.append("possible_page_header")
    return normalized, None, flags


def _chunks(db: sqlite3.Connection, source: dict[str, Any], after: int, limit: int) -> Iterator[dict[str, Any]]:
    if source["source_kind"] == "book":
        sql = """SELECT c.id,c.chunk_index,substr(c.text,1,?) AS text,c.source,
                   c.page_start,c.page_end,c.section_id,s.title AS section_title,s.source_ref
                 FROM chunks c LEFT JOIN book_sections s ON s.id=c.section_id
                 WHERE c.book_id=? AND c.id>? ORDER BY c.id LIMIT ?"""
    else:
        if after:
            previous = db.execute("SELECT chunk_index FROM story_library_chunks WHERE id=? AND source_id=?", (after,source["source_record_id"])).fetchone()
            if not previous:
                raise ValueError("story cursor chunk does not belong to selected source")
            after = int(previous[0])
        else:
            after = -1
        sql = """SELECT id,chunk_index,substr(text,1,?) AS text,section_title,source_ref
                 FROM story_library_chunks WHERE source_id=? AND chunk_index>? ORDER BY chunk_index LIMIT ?"""
    for row in db.execute(sql, (MAX_INPUT_CHARS + 1, source["source_record_id"], after, limit)):
        yield dict(row)


def export_chunks(path: str | Path, dataset_id: str, output: str | Path, *,
                  limit_sources: int = 10, limit_chunks: int = 1000, max_chars: int = 1800,
                  categories: list[str] | None = None, source_ids: list[str] | None = None,
                  cursor: dict[str, Any] | None = None, state_path: str | Path | None = None,
                  cleaning_version: str = CLEANING_VERSION) -> dict[str, Any]:
    if min(limit_sources, limit_chunks, max_chars) <= 0:
        raise ValueError("limits and max-chars must be positive")
    if not _NAMESPACE.fullmatch(cleaning_version):
        raise ValueError("invalid cleaning version")
    sources = [source for source in read_catalog(path, dataset_id)
               if (not source_ids or source["id"] in source_ids)
               and (not categories or source["category"] in categories or
                    any(item["name"] in categories for item in source["metadata"]["categories"]))
               and (bool(source_ids or categories) or source["enabled"])]
    if source_ids and set(source_ids) - {source["id"] for source in sources}:
        raise ValueError("source-id not found in selected dataset/categories")
    start = 0
    if cursor:
        matches = [index for index, source in enumerate(sources) if source["id"] == cursor.get("source_id")]
        if not matches or not isinstance(cursor.get("chunk_id"), int) or cursor["chunk_id"] < 0:
            raise ValueError("cursor does not match the selected dataset/sources")
        start = matches[0]
    output = Path(output)
    if output.exists():
        raise ValueError("output already exists; choose a new batch filename")
    output.parent.mkdir(parents=True, exist_ok=True)
    report: dict[str, Any] = {"dataset_id": dataset_id, "cleaning_version": cleaning_version,
        "consumed_chunks": 0, "exported_chunks": 0, "visited_sources": 0,
        "rejected": {}, "reject_examples": [], "next_cursor": cursor, "complete": True}
    rejects: Counter[str] = Counter()
    with tempfile.TemporaryDirectory(prefix="sqlite-public-") as temporary:
        dedup_path = Path(state_path) if state_path else Path(temporary) / "dedup.sqlite"
        if dedup_path.resolve() == Path(path).resolve() or dedup_path.resolve() == output.resolve():
            raise ValueError("dedup state must be a separate writable file")
        with closing(sqlite3.connect(dedup_path)) as dedup, dedup, open_readonly(path) as db, output.open("x", encoding="utf-8") as stream:
            dedup.execute("CREATE TABLE IF NOT EXISTS seen(source_id TEXT,hash TEXT,version TEXT,PRIMARY KEY(source_id,hash,version))")
            for source_index, source in enumerate(sources[start:], start=start):
                if report["visited_sources"] >= limit_sources or report["consumed_chunks"] >= limit_chunks:
                    report["complete"] = False
                    break
                report["visited_sources"] += 1
                after = cursor["chunk_id"] if cursor and source_index == start else 0
                consumed_here = 0
                remaining = limit_chunks - report["consumed_chunks"]
                for chunk in _chunks(db, source, after, remaining):
                    consumed_here += 1
                    report["consumed_chunks"] += 1
                    report["next_cursor"] = {"source_id": source["id"], "chunk_id": chunk["id"]}
                    content, reason, flags = clean_text(chunk["text"] or "")
                    lines = content.splitlines()
                    if len(lines) > 1 and (lines[0] == source["title"] or lines[-1] == source["title"]):
                        flags.append("possible_title_header")
                    if reason:
                        rejects[reason] += 1
                        if len(report["reject_examples"]) < 20:
                            report["reject_examples"].append({"source_id": source["id"], "chunk_id": chunk["id"], "reason": reason})
                        continue
                    locator = {key: chunk.get(key) for key in ("chunk_index", "page_start", "page_end", "section_id", "section_title", "source_ref") if chunk.get(key) is not None}
                    locator.update({"sqlite_chunk_id": chunk["id"], "original_source_id": source["source_record_id"]})
                    # Limits count input rows; emit all segments before advancing the resumable cursor.
                    for segment_index, offset in enumerate(range(0, len(content), max_chars)):
                        segment = content[offset:offset + max_chars].strip()
                        if not segment:
                            continue
                        digest = hashlib.sha256(segment.encode("utf-8")).hexdigest()
                        inserted = dedup.execute("INSERT OR IGNORE INTO seen VALUES(?,?,?)", (source["id"], digest, cleaning_version)).rowcount
                        if not inserted:
                            rejects["duplicate"] += 1
                            continue
                        row = {"id": f"{source['id']}:chunk:{chunk['id']}:part:{segment_index}:{cleaning_version}",
                            "public_source_id": source["id"], "dataset_id": dataset_id, "title": source["title"],
                            "content": segment, "source": source["title"], "locator": {**locator, "segment_index": segment_index},
                            "metadata": {"category": source["category"], "quality_flags": flags,
                                "cleaning_version": cleaning_version, "source_content_hash": hashlib.sha256((chunk["text"] or "").encode()).hexdigest(),
                                "content_is_untrusted_reference": True},
                            "content_hash": digest, "index_version": cleaning_version,
                            "search_text": search_tokens(source["title"] + "\n" + segment)}
                        stream.write(json.dumps(row, ensure_ascii=False) + "\n")
                        report["exported_chunks"] += 1
                if consumed_here == remaining:
                    # A full SQL page is conservatively resumable, including when exactly at EOF.
                    report["complete"] = False
                    break
                if source_index + 1 < len(sources):
                    report["next_cursor"] = {"source_id": sources[source_index + 1]["id"], "chunk_id": 0}
                elif report["next_cursor"] is None or report["next_cursor"].get("source_id") != source["id"]:
                    report["next_cursor"] = {"source_id": source["id"], "chunk_id": after}
    report["rejected"] = dict(rejects)
    return report


def _jsonl(path: str | Path) -> Iterator[dict[str, Any]]:
    with Path(path).open(encoding="utf-8") as stream:
        for number, line in enumerate(stream, 1):
            if not line.strip():
                continue
            if len(line) > 2_000_000:
                raise ValueError(f"JSONL line {number} exceeds size limit")
            row = json.loads(line)
            if not isinstance(row, dict):
                raise ValueError(f"JSONL line {number} must be an object")
            yield row


def register_catalog(database_url: str, catalog: str | Path) -> dict[str, int]:
    count = 0
    with psycopg.connect(database_url) as db, db.cursor() as cursor:
        for row in _jsonl(catalog):
            _dataset_id(row["dataset_id"])
            if row["source_kind"] not in {"book", "story"} or row["id"] != f"{row['dataset_id']}:{row['source_kind']}:{row['source_record_id']}":
                raise ValueError("invalid catalog source identity")
            cursor.execute("""INSERT INTO public_knowledge_sources
              (id,dataset_id,source_kind,source_record_id,title,category,file_format,extract_status,
               source_chunks,text_chars,enabled,quality_status,metadata)
              VALUES(%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s)
              ON CONFLICT(id) DO UPDATE SET title=EXCLUDED.title,category=EXCLUDED.category,
                file_format=EXCLUDED.file_format,extract_status=EXCLUDED.extract_status,
                source_chunks=EXCLUDED.source_chunks,text_chars=EXCLUDED.text_chars,
                metadata=EXCLUDED.metadata,update_time=now()""",
                tuple(row[key] for key in ("id","dataset_id","source_kind","source_record_id","title","category","file_format","extract_status","source_chunks","text_chars","enabled","quality_status")) + (Jsonb(row.get("metadata", {})),))
            count += 1
    return {"registered": count}


def _file_digest(path: str | Path) -> str:
    digest = hashlib.sha256()
    with Path(path).open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def import_batch(database_url: str, input_path: str | Path, batch_id: str) -> dict[str, int]:
    if not _NAMESPACE.fullmatch(batch_id):
        raise ValueError("invalid batch-id")
    digest = _file_digest(input_path)
    inserted = skipped = 0
    affected: set[str] = set()
    source_datasets: dict[str, str] = {}
    # Preflight retains only source identities; body rows remain streamed on the second pass.
    for row in _jsonl(input_path):
        source_id = row["public_source_id"]
        declared_dataset = _dataset_id(row["dataset_id"])
        if not source_id.startswith(declared_dataset + ":"):
            raise ValueError("document dataset does not match source namespace")
        source_datasets[source_id] = declared_dataset
    with psycopg.connect(database_url) as db, db.cursor() as cursor:
        cursor.execute("SELECT pg_advisory_xact_lock(hashtext(%s))", ("public-import:" + batch_id,))
        cursor.execute("SELECT file_hash,status FROM public_knowledge_import_batches WHERE batch_id=%s FOR UPDATE", (batch_id,))
        existing_batch = cursor.fetchone()
        if existing_batch:
            if existing_batch[1] == "rolled_back":
                raise ValueError("batch-id was rolled back; use a new batch-id")
            if existing_batch[0] != digest:
                raise ValueError("batch-id already belongs to a different immutable import file")
        else:
            cursor.execute("INSERT INTO public_knowledge_import_batches(batch_id,file_hash,status) VALUES(%s,%s,'completed')", (batch_id,digest))
        if source_datasets:
            cursor.execute("SELECT id,dataset_id FROM public_knowledge_sources WHERE id=ANY(%s::text[]) ORDER BY id FOR NO KEY UPDATE", (sorted(source_datasets),))
            registered = dict(cursor.fetchall())
            if registered != source_datasets:
                raise ValueError("sources must be registered in the declared dataset before import")
        for row in _jsonl(input_path):
            source_id = row["public_source_id"]
            declared_dataset = _dataset_id(row["dataset_id"])
            if source_datasets.get(source_id) != declared_dataset:
                raise ValueError("document dataset does not match source namespace")
            if not row["id"].startswith(source_id + ":chunk:") or not row["id"].endswith(":" + row["index_version"]):
                raise ValueError("document id must include source/chunk and cleaning version")
            content_hash = hashlib.sha256(row["content"].encode()).hexdigest()
            if row["content_hash"] != content_hash:
                raise ValueError("document content hash mismatch")
            cursor.execute("SELECT content_hash,public_source_id,index_version FROM knowledge_documents WHERE id=%s", (row["id"],))
            existing = cursor.fetchone()
            if existing:
                if existing != (content_hash, source_id, row["index_version"]):
                    raise ValueError("same document id has changed content; use a new cleaning version")
                skipped += 1
                continue
            metadata = {**row.get("metadata", {}), "import_file_hash": digest, "content_is_untrusted_reference": True}
            search_text = search_tokens(row["title"] + "\n" + row["content"])
            cursor.execute("""INSERT INTO knowledge_documents
                (id,library_kind,release_id,enneagram_type,safety_level,title,content,source,locator,metadata,
                 content_hash,embedding_model,index_version,embedding,public_source_id,import_batch_id,search_text,public_search_vector)
                VALUES(%s,'public',NULL,NULL,0,%s,%s,%s,%s,%s,%s,'',%s,NULL,%s,%s,%s,to_tsvector('simple',%s))
                ON CONFLICT DO NOTHING RETURNING id""",
                (row["id"], row["title"], row["content"], row.get("source", row["title"]), Jsonb(row.get("locator", {})),
                 Jsonb(metadata), content_hash, row["index_version"], source_id, batch_id,
                 search_text, search_text))
            if cursor.fetchone():
                inserted += 1
                affected.add(source_id)
            else:
                cursor.execute("SELECT content_hash,public_source_id,index_version FROM knowledge_documents WHERE id=%s", (row["id"],))
                collision = cursor.fetchone()
                if collision and collision != (content_hash, source_id, row["index_version"]):
                    raise ValueError("concurrent same-id import has changed content")
                skipped += 1
        if _file_digest(input_path) != digest:
            raise ValueError("import input file changed while importing")
        _refresh_counts(cursor, affected)
    return {"inserted": inserted, "skipped": skipped}


def _refresh_counts(cursor, sources: set[str]) -> None:
    if sources:
        cursor.execute("SELECT id FROM public_knowledge_sources WHERE id=ANY(%s::text[]) ORDER BY id FOR NO KEY UPDATE", (sorted(sources),))
        cursor.fetchall()
        cursor.execute("""WITH counts AS (
            SELECT s.id,count(d.id) AS total,
              coalesce(bool_or(jsonb_array_length(coalesce(d.metadata->'quality_flags','[]'::jsonb))>0),false) AS needs_review
            FROM public_knowledge_sources s LEFT JOIN knowledge_documents d ON d.public_source_id=s.id
            WHERE s.id=ANY(%s::text[]) GROUP BY s.id
          ) UPDATE public_knowledge_sources s SET imported_chunks=c.total,
          quality_status=CASE
            WHEN NOT (lower(s.extract_status)=ANY(%s::text[])) OR c.needs_review THEN 'needs_review'
            WHEN c.total=0 THEN 'pending' ELSE 'ready' END,
          update_time=now() FROM counts c WHERE s.id=c.id""", (sorted(sources),list(_NORMAL_EXTRACTION_STATUSES)))


def rollback_batch(database_url: str, batch_id: str) -> dict[str, int]:
    with psycopg.connect(database_url) as db, db.cursor() as cursor:
        cursor.execute("SELECT pg_advisory_xact_lock(hashtext(%s))", ("public-import:" + batch_id,))
        cursor.execute("SELECT DISTINCT public_source_id FROM knowledge_documents WHERE import_batch_id=%s AND public_source_id IS NOT NULL", (batch_id,))
        sources = sorted(row[0] for row in cursor.fetchall())
        if sources:
            cursor.execute("SELECT id FROM public_knowledge_sources WHERE id=ANY(%s::text[]) ORDER BY id FOR NO KEY UPDATE", (sources,))
            cursor.fetchall()
        cursor.execute("DELETE FROM knowledge_documents WHERE import_batch_id=%s AND public_source_id IS NOT NULL RETURNING public_source_id", (batch_id,))
        deleted = cursor.fetchall()
        cursor.execute("UPDATE public_knowledge_import_batches SET status='rolled_back' WHERE batch_id=%s", (batch_id,))
        _refresh_counts(cursor, {row[0] for row in deleted})
    return {"deleted": len(deleted)}


def main(argv: list[str] | None = None) -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    for name in ("catalog", "export"):
        command = commands.add_parser(name)
        command.add_argument("--sqlite", required=True)
        command.add_argument("--dataset-id", required=True)
        command.add_argument("--output", required=True)
        if name == "export":
            command.add_argument("--limit-sources", type=int, default=10)
            command.add_argument("--limit-chunks", type=int, default=1000)
            command.add_argument("--max-chars", type=int, default=1800)
            command.add_argument("--category", action="append")
            command.add_argument("--source-id", action="append")
            command.add_argument("--cursor", help="JSON source_id/chunk_id from previous report")
            command.add_argument("--state")
            command.add_argument("--report")
            command.add_argument("--cleaning-version", default=CLEANING_VERSION)
    for name in ("register", "import", "rollback"):
        command = commands.add_parser(name)
        command.add_argument("--database-url", default=os.getenv("DATABASE_URL"))
        command.add_argument("--catalog" if name == "register" else "--input" if name == "import" else "--batch-id", required=True)
        if name == "import":
            command.add_argument("--batch-id", required=True)
            command.add_argument("--bulk", action="store_true", help="Use COPY staging for large batches")
    args = parser.parse_args(argv)
    if args.command == "catalog":
        counts: Counter[str] = Counter()
        sources = enabled = 0
        output = Path(args.output)
        output.parent.mkdir(parents=True, exist_ok=True)
        with output.open("x", encoding="utf-8") as stream:
            for row in read_catalog(args.sqlite, args.dataset_id):
                stream.write(json.dumps(row, ensure_ascii=False) + "\n")
                sources += 1
                enabled += int(row["enabled"])
                counts[row["category"]] += 1
        result = {"sources": sources, "enabled_by_default": enabled, "categories": dict(counts)}
    elif args.command == "export":
        result = export_chunks(args.sqlite, args.dataset_id, args.output, limit_sources=args.limit_sources,
            limit_chunks=args.limit_chunks, max_chars=args.max_chars, categories=args.category,
            source_ids=args.source_id, cursor=json.loads(args.cursor) if args.cursor else None,
            state_path=args.state, cleaning_version=args.cleaning_version)
        if args.report:
            with Path(args.report).open("x", encoding="utf-8") as stream:
                json.dump(result, stream, ensure_ascii=False, indent=2)
    else:
        if not args.database_url:
            parser.error("--database-url or DATABASE_URL is required for write commands")
        if args.command == "register":
            result = register_catalog(args.database_url, args.catalog)
        elif args.command == "import":
            if args.bulk:
                from app.ingestion.bulk_public import bulk_import_batch
                result = bulk_import_batch(args.database_url, args.input, args.batch_id)
            else:
                result = import_batch(args.database_url, args.input, args.batch_id)
        else:
            result = rollback_batch(args.database_url, args.batch_id)
    print(json.dumps(result, ensure_ascii=False))


if __name__ == "__main__":
    main()
