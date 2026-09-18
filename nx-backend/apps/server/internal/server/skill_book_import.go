package server

import (
	"io"
	"net/http"
	"strconv"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/skillcatalog"
)

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
	result, err := skillcatalog.ImportBook(r.Context(), s.db, skillcatalog.BookImportInput{Name: name, Summary: r.FormValue("summary"), CategoryID: categoryID, Filename: header.Filename, Text: content})
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "书籍入库失败，请确认名称不超过 120 字、简介不超过 1000 字且分类已启用")
		return
	}
	httpx.OK(w, result)
}
