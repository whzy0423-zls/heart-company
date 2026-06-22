package server

import (
	"context"
	"net/http"
	"time"

	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/rag"
)

// retrieveDocsForQuery 选取用于回答某问题的知识文档。
// 当 embedding 已配置且数据库启用 pgvector 时，用查询向量做语义近邻检索（更准）；
// 否则回退到「全部启用文档 + 内存关键词打分」（rag.Service 内部完成）。
//
// 返回的文档会交给 rag.NewService 再做一轮打分/截断，因此向量检索这里
// 取稍宽的候选集（topK 命中），既享受语义召回，又保留原有兜底逻辑。
func (s *Server) retrieveDocsForQuery(ctx context.Context, question string, topK int) ([]rag.Document, error) {
	base, err := s.miniappRAGDocuments(ctx)
	if err != nil {
		return nil, err
	}
	if s.embedder == nil || !s.embedder.Enabled() {
		return base, nil
	}
	if s.ragVec == nil || !s.ragVec.VectorAvailable(ctx) {
		return base, nil
	}
	vector, err := s.embedder.Embed(ctx, question)
	if err != nil {
		// 向量化失败不致命，回退关键词
		return base, nil
	}
	hits, err := s.ragVec.SearchByVector(ctx, vector, topK)
	if err != nil || len(hits) == 0 {
		return base, nil
	}
	// 语义命中放前面，再接上站点文档（站点信息如课程/价格不在知识库里）。
	siteDocs := siteOnlyDocs(base, hits)
	return append(hits, siteDocs...), nil
}

// siteOnlyDocs 返回 base 中不属于知识库命中的文档（按 Title 粗去重），
// 避免站点配置类文档（课程、报名等）被向量检索漏掉。
func siteOnlyDocs(base, hits []rag.Document) []rag.Document {
	seen := make(map[string]bool, len(hits))
	for _, h := range hits {
		seen[h.Title] = true
	}
	out := make([]rag.Document, 0, len(base))
	for _, d := range base {
		if !seen[d.Title] {
			out = append(out, d)
		}
	}
	return out
}

// ragReindex 对启用的知识文档做（增量）向量化。需要后台鉴权。
// 未配置 embedding 或数据库未启用 pgvector 时返回明确提示。
func (s *Server) ragReindex(w http.ResponseWriter, r *http.Request) {
	if s.embedder == nil || !s.embedder.Enabled() {
		httpx.Fail(w, http.StatusBadRequest, "未配置 EMBEDDING_PROVIDER/API_KEY/MODEL，向量化未启用")
		return
	}
	if s.ragVec == nil || !s.ragVec.VectorAvailable(r.Context()) {
		httpx.Fail(w, http.StatusBadRequest, "数据库未启用 pgvector（需使用 pgvector/pgvector 镜像）")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Minute)
	defer cancel()

	model := s.embedder.ModelName()
	pending, err := s.ragVec.DocsNeedingEmbedding(ctx, model, 500)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}

	var done, failed int
	for _, d := range pending {
		text := d.Title + "\n" + d.Content
		vector, err := s.embedder.Embed(ctx, text)
		if err != nil {
			failed++
			continue
		}
		if err := s.ragVec.UpdateEmbedding(ctx, d.ID, vector, model); err != nil {
			failed++
			continue
		}
		done++
	}
	httpx.OK(w, map[string]any{
		"pending": len(pending),
		"done":    done,
		"failed":  failed,
	})
}
