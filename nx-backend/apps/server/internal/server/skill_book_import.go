package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/skillcatalog"
)

func (s *Server) distillImportedBook(r *http.Request, name, text string) (*skillcatalog.BookDistillation, error) {
	// Keep the prompt bounded; the complete book remains available to RAG.
	const maxPromptRunes = 24000
	sample := text
	if utf8.RuneCountInString(sample) > maxPromptRunes {
		runes := []rune(sample)
		sample = string(runes[:maxPromptRunes])
	}
	system := "你是 book-to-skill 蒸馏器。只输出 JSON，不要 Markdown 代码围栏。字段必须是 overviewMarkdown(string)、coreMarkdown(string)、whenToUse(array of 3-6 strings)、workflow(array of 3-6 strings)、topics(array of 3-8 strings)。只能依据给定书籍正文归纳，不编造作者没有表达的结论。"
	user := fmt.Sprintf("书名：%s\n请把以下正文提炼成可在手机端展示和执行的技能摘要：\n%s", name, sample)
	content, err := s.completePreferenceJSON(r.Context(), system, user, 1200)
	if err != nil {
		return nil, fmt.Errorf("当前 AI 生成失败: %w", err)
	}
	var value skillcatalog.BookDistillation
	if err := json.Unmarshal([]byte(content), &value); err != nil {
		return nil, fmt.Errorf("当前 AI 返回的技能结构无效: %w", err)
	}
	if strings.TrimSpace(value.OverviewMarkdown) == "" || strings.TrimSpace(value.CoreMarkdown) == "" || len(value.WhenToUse) < 3 || len(value.Workflow) < 3 {
		return nil, fmt.Errorf("当前 AI 返回的技能缺少核心框架、适用场景或默认工作流")
	}
	return &value, nil
}

func (s *Server) importSkillBook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.Fail(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s == nil || s.db == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "成长技能库服务未配置")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, skillcatalog.MaxBookBytes+(1<<20))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "请上传不超过 200 MB 的书籍文件")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "请选择书籍文件")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, skillcatalog.MaxBookBytes+1))
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "书籍文件读取失败")
		return
	}
	categoryID, _ := strconv.ParseInt(r.FormValue("categoryId"), 10, 64)
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" || categoryID <= 0 {
		httpx.Fail(w, http.StatusBadRequest, "请填写技能名称并选择分类")
		return
	}
	content, err := skillcatalog.ExtractBookText(r.Context(), header.Filename, data)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	distillation, err := s.distillImportedBook(r, name, content)
	if err != nil {
		httpx.Fail(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	result, err := skillcatalog.ImportBook(r.Context(), s.db, skillcatalog.BookImportInput{Name: name, Summary: r.FormValue("summary"), CategoryID: categoryID, Filename: header.Filename, Text: content, Distillation: distillation})
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "书籍入库失败，请确认名称不超过 120 字、简介不超过 1000 字且分类已启用")
		return
	}
	httpx.OK(w, result)
}
