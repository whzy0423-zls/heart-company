package articlestore

import "testing"

func TestNormalizeArticleTrimsAndDefaults(t *testing.T) {
	doc, err := NormalizeArticle(Article{
		Title:   "  九型与亲密关系  ",
		Summary: "  在关系里照见自己  ",
		Content: "  # 标题\n正文内容  ",
		Author:  "  芯之力  ",
		Tags:    []string{" 关系 ", "成长", "关系", ""},
	})
	if err != nil {
		t.Fatalf("NormalizeArticle returned error: %v", err)
	}
	if doc.Title != "九型与亲密关系" {
		t.Fatalf("unexpected title: %q", doc.Title)
	}
	if doc.Summary != "在关系里照见自己" {
		t.Fatalf("unexpected summary: %q", doc.Summary)
	}
	if doc.Author != "芯之力" {
		t.Fatalf("unexpected author: %q", doc.Author)
	}
	if doc.Status != StatusPublished {
		t.Fatalf("expected default status published, got %q", doc.Status)
	}
	if len(doc.Tags) != 2 || doc.Tags[0] != "关系" || doc.Tags[1] != "成长" {
		t.Fatalf("unexpected tags: %+v", doc.Tags)
	}
}

func TestNormalizeArticleKeepsDraftStatus(t *testing.T) {
	doc, err := NormalizeArticle(Article{
		Title:   "草稿",
		Content: "内容",
		Status:  StatusDraft,
	})
	if err != nil {
		t.Fatalf("NormalizeArticle returned error: %v", err)
	}
	if doc.Status != StatusDraft {
		t.Fatalf("expected draft status preserved, got %q", doc.Status)
	}
}

func TestNormalizeArticleRequiresTitleAndContent(t *testing.T) {
	if _, err := NormalizeArticle(Article{Content: "正文"}); err == nil {
		t.Fatal("expected error for missing title")
	}
	if _, err := NormalizeArticle(Article{Title: "标题"}); err == nil {
		t.Fatal("expected error for missing content")
	}
}
