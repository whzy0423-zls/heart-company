from app.ingestion.chapter_splitter import split_chapters
from app.ingestion.metadata import ExtractedSection
from app.ingestion.semantic_splitter import split_sections


def test_chapter_splitter_keeps_headings_and_kinds() -> None:
    text = "第一章 基础\n普通说明。\n定义：人格是一种模式。\n案例：小王开始观察自己。\n警告：不要给他人贴标签。"

    sections = split_chapters(text)

    assert sections[0].title == "第一章 基础"
    assert {section.kind for section in sections} >= {"body", "definition", "case", "warning"}


def test_semantic_splitter_has_bounded_size_overlap_and_stable_hash() -> None:
    text = "".join(f"第{i}段内容用于测试语义切分。" for i in range(160))
    source = ExtractedSection(text=text, locator={"chapter": 1}, title="测试章")

    first = split_sections([source], target_chars=700, max_chars=900, overlap_chars=120)
    second = split_sections([source], target_chars=700, max_chars=900, overlap_chars=120)

    assert len(first) > 1
    assert all(len(chunk.text) <= 900 for chunk in first)
    assert first[0].text[-80:] in first[1].text
    assert [chunk.content_hash for chunk in first] == [chunk.content_hash for chunk in second]
    assert all(chunk.locator == {"chapter": 1} for chunk in first)
