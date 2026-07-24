package theorystore

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"nine-xing/nx-backend/apps/server/internal/rag"
)

const maxTheorySearchCandidates = 80

type scoredTheoryDocument struct {
	document rag.Document
	score    float64
}

// SearchActiveChunks searches only chunks frozen into each library's current
// active release. The score is lexical and explainable: title, keywords and
// body contribute independently, and minScore is applied after normalization.
func (s *Store) SearchActiveChunks(parent context.Context, query string, topK int, minScore float64) ([]rag.Document, error) {
	if err := s.available(); err != nil {
		return nil, err
	}
	query = normalizeSearchText(query)
	if query == "" || topK <= 0 {
		return nil, nil
	}
	if topK > 20 {
		topK = 20
	}
	if minScore < 0 {
		minScore = 0
	}
	if minScore > 1 {
		minScore = 1
	}
	ctx, cancel := storeContext(parent)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT chunk.id, chunk.title, chunk.content, chunk.keywords, chunk.tags
		FROM theory_libraries library
		JOIN theory_library_releases release
		  ON release.library_id = library.id
		 AND release.version = library.current_version
		 AND release.status = 'active'
		JOIN theory_release_cards mapping ON mapping.release_id = release.id
		JOIN theory_chunks chunk ON chunk.id = mapping.chunk_id
		WHERE library.status = 'enabled'
		  AND chunk.status = 'enabled'
		  AND (chunk.title ILIKE '%' || $1 || '%'
		       OR chunk.content ILIKE '%' || $1 || '%'
		       OR chunk.keywords::text ILIKE '%' || $1 || '%'
		       OR chunk.tags::text ILIKE '%' || $1 || '%'
		       OR char_length($1) >= 2)
		ORDER BY chunk.id
		LIMIT $2`, query, maxTheorySearchCandidates)
	if err != nil {
		return nil, fmt.Errorf("search active theory chunks: %w", err)
	}
	defer rows.Close()

	matches := make([]scoredTheoryDocument, 0, topK)
	for rows.Next() {
		var id int64
		var title, content string
		var keywordsRaw, tagsRaw []byte
		if err := rows.Scan(&id, &title, &content, &keywordsRaw, &tagsRaw); err != nil {
			return nil, fmt.Errorf("search active theory chunks: scan: %w", err)
		}
		keywords := decodeStringList(keywordsRaw)
		tags := decodeStringList(tagsRaw)
		score := theoryLexicalScore(query, title, content, keywords)
		if score < minScore {
			continue
		}
		matches = append(matches, scoredTheoryDocument{document: rag.Document{
			ID: fmt.Sprintf("theory:%d", id), Title: strings.TrimSpace(title),
			Content: strings.TrimSpace(content), Tags: tags,
		}, score: score})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search active theory chunks: rows: %w", err)
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].score == matches[j].score {
			return matches[i].document.ID < matches[j].document.ID
		}
		return matches[i].score > matches[j].score
	})
	if len(matches) > topK {
		matches = matches[:topK]
	}
	documents := make([]rag.Document, len(matches))
	for i := range matches {
		documents[i] = matches[i].document
	}
	return documents, nil
}

func theoryLexicalScore(query, title, content string, keywords []string) float64 {
	query = normalizeSearchText(query)
	if query == "" {
		return 0
	}
	title = normalizeSearchText(title)
	content = normalizeSearchText(content)
	score := 0.0
	if strings.Contains(title, query) || strings.Contains(query, title) && len([]rune(title)) >= 2 {
		score += 0.55
	} else if overlapRatio(queryBigrams(query), queryBigrams(title)) >= 0.25 {
		score += 0.30
	}
	keywordMatches := 0
	for _, keyword := range keywords {
		keyword = normalizeSearchText(keyword)
		if len([]rune(keyword)) >= 2 && (strings.Contains(query, keyword) || strings.Contains(keyword, query)) {
			keywordMatches++
		}
	}
	if keywordMatches > 0 {
		score += 0.35
		if keywordMatches > 1 {
			score += 0.10
		}
	}
	bodyOverlap := overlapRatio(queryBigrams(query), queryBigrams(content))
	if bodyOverlap >= 0.12 {
		score += 0.20
	} else if bodyOverlap >= 0.06 {
		score += 0.10
	}
	if score > 1 {
		return 1
	}
	return score
}

func normalizeSearchText(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			return -1
		}
		return unicode.ToLower(r)
	}, strings.TrimSpace(value))
}

func queryBigrams(value string) map[string]struct{} {
	runes := []rune(normalizeSearchText(value))
	grams := make(map[string]struct{})
	if len(runes) == 1 {
		grams[string(runes)] = struct{}{}
	}
	for i := 0; i+1 < len(runes); i++ {
		grams[string(runes[i:i+2])] = struct{}{}
	}
	return grams
}

func overlapRatio(query, candidate map[string]struct{}) float64 {
	if len(query) == 0 || len(candidate) == 0 {
		return 0
	}
	matches := 0
	for gram := range query {
		if _, ok := candidate[gram]; ok {
			matches++
		}
	}
	return float64(matches) / float64(len(query))
}

func decodeStringList(raw []byte) []string {
	var values []string
	if len(raw) == 0 || json.Unmarshal(raw, &values) != nil {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}
