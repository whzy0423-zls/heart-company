import json
import zipfile
from pathlib import Path

from app.ingestion.catalog import build_catalog, write_catalog


def test_catalog_marks_exact_duplicates_without_deleting_files(tmp_path: Path) -> None:
    first = tmp_path / "心理学.pdf"
    duplicate = tmp_path / "心理学-副本.pdf"
    first.write_bytes(b"same-book")
    duplicate.write_bytes(b"same-book")

    entries = build_catalog(tmp_path)

    assert [entry.status for entry in entries] == ["canonical", "duplicate"]
    assert entries[1].canonical_id == entries[0].id
    assert first.exists() and duplicate.exists()


def test_catalog_groups_pdf_and_epub_editions_by_normalized_title(tmp_path: Path) -> None:
    (tmp_path / "亲密关系.pdf").write_bytes(b"pdf-version")
    with zipfile.ZipFile(tmp_path / "亲密关系.epub", "w") as archive:
        archive.writestr("mimetype", "application/epub+zip")

    entries = build_catalog(tmp_path)

    assert len({entry.work_id for entry in entries}) == 1
    assert sum(entry.status == "canonical" for entry in entries) == 1
    assert sum(entry.status == "duplicate" for entry in entries) == 1


def test_catalog_rejects_empty_executable_partial_and_corrupt_archive(tmp_path: Path) -> None:
    (tmp_path / "empty.pdf").write_bytes(b"")
    (tmp_path / "program.exe").write_bytes(b"MZ")
    (tmp_path / "book.pdf.part").write_bytes(b"partial")
    (tmp_path / "broken.epub").write_bytes(b"not-a-zip")

    entries = build_catalog(tmp_path)

    assert {entry.status for entry in entries} == {"rejected"}
    assert {entry.reason for entry in entries} == {
        "empty_file",
        "unsupported_extension",
        "incomplete_download",
        "corrupt_archive",
    }


def test_catalog_jsonl_is_stable(tmp_path: Path) -> None:
    source = tmp_path / "books"
    source.mkdir()
    (source / "B.txt").write_text("b", encoding="utf-8")
    (source / "A.txt").write_text("a", encoding="utf-8")
    first = tmp_path / "first.jsonl"
    second = tmp_path / "second.jsonl"

    write_catalog(build_catalog(source), first)
    write_catalog(build_catalog(source), second)

    assert first.read_bytes() == second.read_bytes()
    rows = [json.loads(line) for line in first.read_text().splitlines()]
    assert [Path(row["path"]).name for row in rows] == ["A.txt", "B.txt"]
