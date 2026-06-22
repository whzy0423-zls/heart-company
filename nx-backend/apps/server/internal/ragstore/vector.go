// 向量检索支持（pgvector）。当数据库未启用 vector 扩展或未配置 embedding 时，
// 这些方法的调用方应回退到关键词检索（EnabledDocuments + 内存打分）。
package ragstore

import (
	"context"
	"strconv"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/rag"
)

// VectorAvailable 检测数据库是否启用了 pgvector 且 rag_documents 已有 embedding 列。
func (s *Store) VectorAvailable(ctx context.Context) bool {
	if s == nil || s.db == nil {
		return false
	}
	c, cancel := s.ctx(ctx)
	defer cancel()
	var exists bool
	err := s.db.QueryRowContext(c,
		`SELECT EXISTS (
		   SELECT 1 FROM information_schema.columns
		   WHERE table_name='rag_documents' AND column_name='embedding'
		 )`,
	).Scan(&exists)
	return err == nil && exists
}

// PendingEmbeddingDoc 待向量化的文档（启用但尚无 embedding，或模型已变更）。
type PendingEmbeddingDoc struct {
	ID      string
	Title   string
	Content string
}

// DocsNeedingEmbedding 取需要（重新）向量化的启用文档。
func (s *Store) DocsNeedingEmbedding(ctx context.Context, model string, limit int) ([]PendingEmbeddingDoc, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()
	rows, err := s.db.QueryContext(c,
		`SELECT id::text, title, content
		   FROM rag_documents
		  WHERE status=$1
		    AND (embedding IS NULL OR embedding_model <> $2)
		  ORDER BY update_time DESC
		  LIMIT $3`,
		StatusEnabled, model, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PendingEmbeddingDoc
	for rows.Next() {
		var d PendingEmbeddingDoc
		if err := rows.Scan(&d.ID, &d.Title, &d.Content); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// UpdateEmbedding 写回某文档的向量与模型标记。
func (s *Store) UpdateEmbedding(ctx context.Context, id string, vector []float32, model string) error {
	c, cancel := s.ctx(ctx)
	defer cancel()
	nid, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(c,
		`UPDATE rag_documents
		    SET embedding=$1::vector, embedding_model=$2, embedded_at=now()
		  WHERE id=$3`,
		vectorLiteral(vector), model, nid,
	)
	return err
}

// SearchByVector 用查询向量做余弦近邻检索，返回最相近的启用文档。
func (s *Store) SearchByVector(ctx context.Context, vector []float32, limit int) ([]rag.Document, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()
	rows, err := s.db.QueryContext(c,
		`SELECT id::text, title, content, tags, status, source, sort, create_time, update_time
		   FROM rag_documents
		  WHERE status=$1 AND embedding IS NOT NULL
		  ORDER BY embedding <=> $2::vector
		  LIMIT $3`,
		StatusEnabled, vectorLiteral(vector), limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanDocuments(rows)
	if err != nil {
		return nil, err
	}
	return ToRAGDocuments(items), nil
}

// vectorLiteral 把 []float32 序列化成 pgvector 文本字面量：[0.1,0.2,...]。
func vectorLiteral(vector []float32) string {
	if len(vector) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.WriteByte('[')
	for i, v := range vector {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(v), 'f', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}
