package server

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auditlog"
	"nine-xing/nx-backend/apps/server/internal/httpx"
)

type theoryLibraryAdminService interface {
	Dashboard(context.Context) (theoryLibraryDashboard, error)
	Cards(context.Context, int64) ([]theoryLibraryCardView, error)
	Publish(context.Context, int64, int64) (theoryLibraryPublishResult, error)
}

type theoryLibraryAdminStore struct{ db *sql.DB }

type theoryLibraryDashboard struct {
	Libraries []theoryLibrarySummary `json:"libraries"`
}

type theoryLibrarySummary struct {
	ID             int64      `json:"id"`
	Key            string     `json:"key"`
	Name           string     `json:"name"`
	Description    string     `json:"description"`
	Status         string     `json:"status"`
	CurrentVersion int        `json:"currentVersion"`
	CardCount      int        `json:"cardCount"`
	PublishedCards int        `json:"publishedCards"`
	ChunkCount     int        `json:"chunkCount"`
	ActiveRelease  *int64     `json:"activeReleaseId"`
	ActivatedAt    *time.Time `json:"activatedAt"`
}

type theoryLibraryCardView struct {
	ID            int64     `json:"id"`
	CanonicalKey  string    `json:"canonicalKey"`
	CanonicalName string    `json:"canonicalName"`
	Domain        string    `json:"domain"`
	CardKind      string    `json:"cardKind"`
	Summary       string    `json:"summary"`
	Status        string    `json:"status"`
	Version       int       `json:"version"`
	UpdateTime    time.Time `json:"updateTime"`
}

type theoryLibraryPublishResult struct {
	LibraryID      int64 `json:"libraryId"`
	ReleaseID      int64 `json:"releaseId"`
	ReleaseVersion int   `json:"releaseVersion"`
	CardCount      int   `json:"cardCount"`
	ChunkCount     int   `json:"chunkCount"`
}

func (s theoryLibraryAdminStore) Dashboard(ctx context.Context) (theoryLibraryDashboard, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT l.id,l.key,l.name,l.description,l.status,l.current_version,
			count(DISTINCT c.id),count(DISTINCT c.id) FILTER (WHERE c.status='published'),
			count(DISTINCT ch.id) FILTER (WHERE ch.status='enabled'),r.id,r.activated_at
		FROM theory_libraries l
		LEFT JOIN theory_cards c ON c.library_id=l.id
		LEFT JOIN theory_chunks ch ON ch.library_id=l.id
		LEFT JOIN theory_library_releases r ON r.library_id=l.id AND r.status='active'
		GROUP BY l.id,r.id,r.activated_at ORDER BY l.id`)
	if err != nil {
		return theoryLibraryDashboard{}, err
	}
	defer rows.Close()
	result := theoryLibraryDashboard{Libraries: []theoryLibrarySummary{}}
	for rows.Next() {
		var item theoryLibrarySummary
		if err := rows.Scan(&item.ID, &item.Key, &item.Name, &item.Description, &item.Status, &item.CurrentVersion, &item.CardCount, &item.PublishedCards, &item.ChunkCount, &item.ActiveRelease, &item.ActivatedAt); err != nil {
			return theoryLibraryDashboard{}, err
		}
		result.Libraries = append(result.Libraries, item)
	}
	return result, rows.Err()
}

func (s theoryLibraryAdminStore) Cards(ctx context.Context, libraryID int64) ([]theoryLibraryCardView, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,canonical_key,canonical_name,domain,card_kind,summary,status,version,update_time FROM theory_cards WHERE library_id=$1 ORDER BY domain,canonical_key,id`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []theoryLibraryCardView{}
	for rows.Next() {
		var item theoryLibraryCardView
		if err := rows.Scan(&item.ID, &item.CanonicalKey, &item.CanonicalName, &item.Domain, &item.CardKind, &item.Summary, &item.Status, &item.Version, &item.UpdateTime); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s theoryLibraryAdminStore) Publish(ctx context.Context, libraryID, actorID int64) (theoryLibraryPublishResult, error) {
	if libraryID <= 0 || actorID <= 0 {
		return theoryLibraryPublishResult{}, errors.New("理论库或管理员信息无效")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return theoryLibraryPublishResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var currentVersion int
	if err := tx.QueryRowContext(ctx, `SELECT current_version FROM theory_libraries WHERE id=$1 FOR UPDATE`, libraryID).Scan(&currentVersion); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return theoryLibraryPublishResult{}, errors.New("理论库不存在")
		}
		return theoryLibraryPublishResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE theory_cards SET status='published',reviewed_by=COALESCE(reviewed_by,$2),reviewed_at=COALESCE(reviewed_at,now()),published_at=COALESCE(published_at,now()),updated_by=$2,update_time=now() WHERE library_id=$1 AND status IN ('draft','in_review')`, libraryID, actorID); err != nil {
		return theoryLibraryPublishResult{}, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,canonical_key,canonical_name,domain,card_kind,summary,definition,core_claim,mechanism,applicable_context,shadow_or_risk,growth_direction,authority_level,evidence_level,clinical_safety,version FROM theory_cards WHERE library_id=$1 AND status='published' ORDER BY id`, libraryID)
	if err != nil {
		return theoryLibraryPublishResult{}, err
	}
	type publishCard struct {
		id                                                                          int64
		authority, version                                                          int
		key, name, domain, kind, summary, definition, claim, mechanism, contextText string
		risk, growth, evidence, safety                                              string
	}
	publishCards := []publishCard{}
	for rows.Next() {
		var card publishCard
		if err := rows.Scan(&card.id, &card.key, &card.name, &card.domain, &card.kind, &card.summary, &card.definition, &card.claim, &card.mechanism, &card.contextText, &card.risk, &card.growth, &card.authority, &card.evidence, &card.safety, &card.version); err != nil {
			rows.Close()
			return theoryLibraryPublishResult{}, err
		}
		publishCards = append(publishCards, card)
	}
	if err := rows.Close(); err != nil {
		return theoryLibraryPublishResult{}, err
	}
	type mapping struct{ cardID, chunkID int64 }
	mappings := []mapping{}
	for _, card := range publishCards {
		content := buildTheoryChunkContent(card.name, card.summary, card.definition, card.claim, card.mechanism, card.contextText, card.risk, card.growth)
		digest := sha256.Sum256([]byte(content))
		var chunkID int64
		err := tx.QueryRowContext(ctx, `INSERT INTO theory_chunks(library_id,card_id,chunk_key,chunk_kind,title,content,keywords,tags,authority_level,evidence_level,clinical_safety,token_count,content_hash,version,status) VALUES($1,$2,$3::text,'card',$4,$5,jsonb_build_array($3::text,$4::text,$6::text),jsonb_build_array('xinzhili','theory-library'),$7,$8,$9,$10,$11,$12,'enabled') ON CONFLICT (library_id,chunk_key,version) DO UPDATE SET card_id=EXCLUDED.card_id,title=EXCLUDED.title,content=EXCLUDED.content,keywords=EXCLUDED.keywords,tags=EXCLUDED.tags,authority_level=EXCLUDED.authority_level,evidence_level=EXCLUDED.evidence_level,clinical_safety=EXCLUDED.clinical_safety,token_count=EXCLUDED.token_count,content_hash=EXCLUDED.content_hash,status='enabled',update_time=now() RETURNING id`, libraryID, card.id, card.key, card.name, content, card.domain, card.authority, card.evidence, card.safety, len([]rune(content)), hex.EncodeToString(digest[:]), card.version).Scan(&chunkID)
		if err != nil {
			return theoryLibraryPublishResult{}, err
		}
		mappings = append(mappings, mapping{cardID: card.id, chunkID: chunkID})
	}
	if len(mappings) == 0 {
		return theoryLibraryPublishResult{}, errors.New("理论库内没有可发布的卡片")
	}
	nextVersion := currentVersion + 1
	var releaseID int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO theory_library_releases(library_id,version,status,retrieval_mode,index_version,card_count,chunk_count) VALUES($1,$2,'ready','lexical_only',$3,$4,$4) RETURNING id`, libraryID, nextVersion, fmt.Sprintf("admin-%d", time.Now().Unix()), len(mappings)).Scan(&releaseID); err != nil {
		return theoryLibraryPublishResult{}, err
	}
	for _, item := range mappings {
		if _, err := tx.ExecContext(ctx, `INSERT INTO theory_release_cards(release_id,card_id,chunk_id) VALUES($1,$2,$3)`, releaseID, item.cardID, item.chunkID); err != nil {
			return theoryLibraryPublishResult{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE theory_library_releases SET status='retired',update_time=now() WHERE library_id=$1 AND status='active'`, libraryID); err != nil {
		return theoryLibraryPublishResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE theory_library_releases SET status='active',activated_by=$2,activated_at=now(),update_time=now() WHERE id=$1 AND status='ready'`, releaseID, actorID); err != nil {
		return theoryLibraryPublishResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE theory_libraries SET status='enabled',current_version=$2,updated_by=$3,update_time=now() WHERE id=$1`, libraryID, nextVersion, actorID); err != nil {
		return theoryLibraryPublishResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return theoryLibraryPublishResult{}, err
	}
	return theoryLibraryPublishResult{LibraryID: libraryID, ReleaseID: releaseID, ReleaseVersion: nextVersion, CardCount: len(mappings), ChunkCount: len(mappings)}, nil
}

func buildTheoryChunkContent(parts ...string) string {
	names := []string{"标题", "摘要", "定义", "核心观点", "作用机制", "适用场景", "风险提示", "成长方向"}
	lines := make([]string, 0, len(parts))
	for index, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			lines = append(lines, names[index]+"："+value)
		}
	}
	return strings.Join(lines, "\n")
}

func (s *Server) theoryLibrariesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}
	result, err := s.theoryAdmin.Dashboard(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, result)
}

func (s *Server) theoryLibraryActionHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/theory-libraries/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 {
		httpx.Fail(w, http.StatusNotFound, "Not Found")
		return
	}
	libraryID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || libraryID <= 0 {
		httpx.Fail(w, http.StatusBadRequest, "理论库 ID 无效")
		return
	}
	switch parts[1] {
	case "cards":
		if r.Method != http.MethodGet {
			httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
			return
		}
		items, err := s.theoryAdmin.Cards(r.Context(), libraryID)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, items)
	case "publish":
		if r.Method != http.MethodPost {
			httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
			return
		}
		result, err := s.theoryAdmin.Publish(r.Context(), libraryID, userFromRequest(r).ID)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		s.recordAdminAudit(r, auditlog.Entry{Action: "theory_library.publish", TargetType: "theory_library", TargetID: strconv.FormatInt(libraryID, 10), Summary: "生成并发布理论库"})
		httpx.OK(w, result)
	default:
		httpx.Fail(w, http.StatusNotFound, "Not Found")
	}
}
