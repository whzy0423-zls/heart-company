package skillcatalog

import (
	"archive/zip"
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestExtractBookText(t *testing.T) {
	content := strings.Repeat("学习需要练习，也需要对结果进行反思。", 8)
	for _, name := range []string{"book.txt", "book.md"} {
		got, err := ExtractBookText(context.Background(), name, []byte("\xef\xbb\xbf"+content))
		if err != nil || got != content {
			t.Fatalf("%s got=%q err=%v", name, got, err)
		}
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	part, _ := writer.Create("word/document.xml")
	part.Write([]byte(`<w:document xmlns:w="urn:test"><w:body><w:p><w:r><w:t>` + content + `</w:t></w:r></w:p></w:body></w:document>`))
	writer.Close()
	got, err := ExtractBookText(context.Background(), "book.docx", buffer.Bytes())
	if err != nil || !strings.Contains(got, content) {
		t.Fatalf("docx got=%q err=%v", got, err)
	}
	for _, data := range [][]byte{[]byte(" "), {0xff, 0xfe, 0x00}, []byte("short")} {
		if _, err := ExtractBookText(context.Background(), "book.txt", data); err == nil {
			t.Fatal("invalid or empty source accepted")
		}
	}
	if _, err := ExtractBookText(context.Background(), "book.exe", []byte(content)); err == nil {
		t.Fatal("unsupported file accepted")
	}
}

func TestExtractBookEPUBSpineOrder(t *testing.T) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entries := map[string]string{
		"META-INF/container.xml": `<container><rootfiles><rootfile full-path="OPS/content.opf"/></rootfiles></container>`,
		"OPS/content.opf":        `<package><manifest><item id="a" href="a.xhtml"/><item id="b" href="b.xhtml"/></manifest><spine><itemref idref="b"/><itemref idref="a"/></spine></package>`,
		"OPS/a.xhtml":            `<html><body><p>` + strings.Repeat("后面的章节。", 20) + `</p></body></html>`,
		"OPS/b.xhtml":            `<html><body><p>` + strings.Repeat("前面的章节。", 20) + `</p></body></html>`,
	}
	for name, value := range entries {
		f, _ := writer.Create(name)
		f.Write([]byte(value))
	}
	writer.Close()
	got, err := ExtractBookText(context.Background(), "book.epub", buffer.Bytes())
	if err != nil || strings.Index(got, "前面的") > strings.Index(got, "后面的") {
		t.Fatalf("order got=%q err=%v", got, err)
	}
}

func TestBookImportRejectsZipExpansionAndCorruptArchives(t *testing.T) {
	if _, err := ExtractBookText(context.Background(), "bad.epub", []byte("broken archive")); err == nil {
		t.Fatal("corrupt EPUB accepted")
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	part, _ := writer.Create("word/document.xml")
	part.Write([]byte(strings.Repeat("x", maxBookTextBytes+1)))
	writer.Close()
	if _, err := ExtractBookText(context.Background(), "large.docx", buffer.Bytes()); err == nil {
		t.Fatal("oversized uncompressed file accepted")
	}
	if _, err := ExtractBookText(context.Background(), "bad.pdf", []byte("not PDF")); err == nil {
		t.Fatal("invalid PDF accepted")
	}
}
