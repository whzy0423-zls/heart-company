package appknowledge

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
)

const (
	LayerPublic        = "public"
	LayerTheory        = "theory"
	LayerEnneagramType = "enneagram_type"
)

type Binding struct {
	Layer         string `json:"layer"`
	EnneagramType *int   `json:"enneagramType,omitempty"`
	LibraryID     int64  `json:"libraryId"`
	LibraryKey    string `json:"libraryKey"`
	ReleaseID     int64  `json:"releaseId"`
}

type Diagnostic struct {
	Layer string `json:"layer"`
	Code  string `json:"code"`
}

type Resolution struct {
	Theory                *Binding     `json:"theory,omitempty"`
	EnneagramType         *Binding     `json:"enneagramType,omitempty"`
	RequestedTypeBindings []*Binding   `json:"requestedTypeBindings,omitempty"`
	Diagnostics           []Diagnostic `json:"diagnostics,omitempty"`
}

type Resolver struct{ db *sql.DB }

func NewResolver(db *sql.DB) *Resolver { return &Resolver{db: db} }

var ErrConversationNotFound = errors.New("app knowledge conversation not found")

type bindingRow struct {
	Layer         string
	EnneagramType *int
	LibraryID     int64
	LibraryKey    string
	LibraryStatus string
	ReleaseID     int64
	ReleaseStatus string
}

func (r *Resolver) Resolve(ctx context.Context, mainType int) (resolution Resolution, retErr error) {
	if r == nil || r.db == nil {
		return Resolution{}, errors.New("app knowledge resolver unavailable")
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return Resolution{}, fmt.Errorf("resolve app knowledge: begin snapshot: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) && retErr == nil {
			retErr = fmt.Errorf("resolve app knowledge: rollback snapshot: %w", err)
		}
	}()
	resolution, err = queryBindings(ctx, tx, mainType, nil)
	if err != nil {
		return Resolution{}, err
	}
	if err := tx.Commit(); err != nil {
		return Resolution{}, fmt.Errorf("resolve app knowledge: commit snapshot: %w", err)
	}
	return resolution, nil
}

func (r *Resolver) ResolveConversation(ctx context.Context, userID, sessionID, cardID int64, requestedTypes []int) (resolution ConversationResolution, retErr error) {
	if r == nil || r.db == nil {
		return ConversationResolution{}, errors.New("app knowledge resolver unavailable")
	}
	if userID <= 0 || sessionID <= 0 || cardID <= 0 {
		return ConversationResolution{}, ErrConversationNotFound
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return ConversationResolution{}, fmt.Errorf("resolve app knowledge conversation: begin snapshot: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) && retErr == nil {
			retErr = fmt.Errorf("resolve app knowledge conversation: rollback snapshot: %w", err)
		}
	}()
	if err := tx.QueryRowContext(ctx, `
		SELECT card.id,card.enneagram,card.revision
		FROM app_chat_sessions session
		JOIN app_user_cards card ON card.id=session.card_id
		WHERE session.id=$1 AND session.app_user_id=$2 AND session.card_id=$3
			AND card.app_user_id=$2 AND card.status='active'`,
		sessionID, userID, cardID,
	).Scan(&resolution.CardID, &resolution.MainType, &resolution.CardRevision); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ConversationResolution{}, ErrConversationNotFound
		}
		return ConversationResolution{}, fmt.Errorf("resolve app knowledge conversation: query card: %w", err)
	}
	resolution.Resolution, err = queryBindings(ctx, tx, resolution.MainType, requestedTypes)
	if err != nil {
		return ConversationResolution{}, err
	}
	if err := tx.Commit(); err != nil {
		return ConversationResolution{}, fmt.Errorf("resolve app knowledge conversation: commit snapshot: %w", err)
	}
	return resolution, nil
}

type bindingQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func queryBindings(ctx context.Context, queryer bindingQueryer, mainType int, requestedTypes []int) (Resolution, error) {
	normalizedTypes := normalizeRequestedTypes(requestedTypes)
	query := `
		SELECT binding.layer_kind,binding.enneagram_type,library.id,library.key,library.status,
			release.id,release.status
		FROM app_chat_knowledge_bindings binding
		JOIN theory_libraries library ON library.id=binding.theory_library_id
		LEFT JOIN theory_library_releases release
			ON release.library_id=library.id AND release.version=library.current_version
		WHERE binding.status='enabled' AND `
	args := make([]any, 0, len(normalizedTypes))
	if len(normalizedTypes) == 0 {
		query += `(binding.layer_kind='theory'
				OR ($1 BETWEEN 1 AND 9 AND binding.layer_kind='enneagram_type' AND binding.enneagram_type=$1))`
		args = append(args, mainType)
	} else {
		placeholders := make([]string, len(normalizedTypes))
		for index, typeNumber := range normalizedTypes {
			placeholders[index] = fmt.Sprintf("$%d", index+1)
			args = append(args, typeNumber)
		}
		query += fmt.Sprintf(`(binding.layer_kind='theory'
				OR (binding.layer_kind='enneagram_type' AND binding.enneagram_type IN (%s)))`, strings.Join(placeholders, ","))
	}
	query += `
		ORDER BY CASE WHEN binding.layer_kind='theory' THEN 0 ELSE 1 END,
			binding.enneagram_type,binding.sort_order,binding.id`
	rows, err := queryer.QueryContext(ctx, query, args...)
	if err != nil {
		return Resolution{}, fmt.Errorf("resolve app knowledge: query bindings: %w", err)
	}
	var bindingRows []bindingRow
	for rows.Next() {
		var row bindingRow
		var personalityType sql.NullInt64
		var releaseID sql.NullInt64
		var releaseStatus sql.NullString
		if err := rows.Scan(&row.Layer, &personalityType, &row.LibraryID, &row.LibraryKey, &row.LibraryStatus, &releaseID, &releaseStatus); err != nil {
			rows.Close()
			return Resolution{}, fmt.Errorf("resolve app knowledge: scan binding: %w", err)
		}
		if personalityType.Valid {
			value := int(personalityType.Int64)
			row.EnneagramType = &value
		}
		row.ReleaseID = releaseID.Int64
		row.ReleaseStatus = releaseStatus.String
		bindingRows = append(bindingRows, row)
	}
	if err := rows.Close(); err != nil {
		return Resolution{}, fmt.Errorf("resolve app knowledge: close rows: %w", err)
	}
	return resolveBindingRows(mainType, normalizedTypes, bindingRows), nil
}

func resolveBindingRows(mainType int, requestedTypes []int, rows []bindingRow) Resolution {
	resolved := Resolution{}
	normalizedTypes := normalizeRequestedTypes(requestedTypes)
	explicitTypes := len(normalizedTypes) > 0
	requestedSet := make(map[int]struct{}, len(normalizedTypes))
	for _, typeNumber := range normalizedTypes {
		requestedSet[typeNumber] = struct{}{}
	}
	requestedBindings := make(map[int]*Binding, len(normalizedTypes))
	validMainType := mainType >= 1 && mainType <= 9
	for _, row := range rows {
		diagnosticLayer := row.Layer
		if explicitTypes && row.Layer == LayerEnneagramType && row.EnneagramType != nil {
			diagnosticLayer = typeTraceKey(*row.EnneagramType)
		}
		if row.LibraryStatus != "enabled" {
			resolved.Diagnostics = append(resolved.Diagnostics, Diagnostic{Layer: diagnosticLayer, Code: "library_disabled"})
			continue
		}
		if row.ReleaseID <= 0 || row.ReleaseStatus != "active" {
			resolved.Diagnostics = append(resolved.Diagnostics, Diagnostic{Layer: diagnosticLayer, Code: "release_not_ready"})
			continue
		}
		binding := &Binding{
			Layer: row.Layer, EnneagramType: row.EnneagramType, LibraryID: row.LibraryID,
			LibraryKey: row.LibraryKey, ReleaseID: row.ReleaseID,
		}
		switch row.Layer {
		case LayerTheory:
			if row.EnneagramType != nil || resolved.Theory != nil {
				resolved.Diagnostics = append(resolved.Diagnostics, Diagnostic{Layer: row.Layer, Code: "invalid_theory_binding"})
				continue
			}
			resolved.Theory = binding
		case LayerEnneagramType:
			if explicitTypes {
				if row.EnneagramType == nil {
					resolved.Diagnostics = append(resolved.Diagnostics, Diagnostic{Layer: LayerEnneagramType, Code: "cross_type_binding"})
					continue
				}
				typeNumber := *row.EnneagramType
				if _, requested := requestedSet[typeNumber]; !requested || row.LibraryKey != fmt.Sprintf("enneagram-type-%02d", typeNumber) {
					resolved.Diagnostics = append(resolved.Diagnostics, Diagnostic{Layer: typeTraceKey(typeNumber), Code: "cross_type_binding"})
					continue
				}
				if requestedBindings[typeNumber] != nil {
					resolved.Diagnostics = append(resolved.Diagnostics, Diagnostic{Layer: typeTraceKey(typeNumber), Code: "duplicate_type_binding"})
					continue
				}
				requestedBindings[typeNumber] = binding
				continue
			}
			if !validMainType || row.EnneagramType == nil || *row.EnneagramType != mainType ||
				row.LibraryKey != fmt.Sprintf("enneagram-type-%02d", mainType) {
				resolved.Diagnostics = append(resolved.Diagnostics, Diagnostic{Layer: row.Layer, Code: "cross_type_binding"})
				continue
			}
			if resolved.EnneagramType != nil {
				resolved.Diagnostics = append(resolved.Diagnostics, Diagnostic{Layer: row.Layer, Code: "duplicate_type_binding"})
				continue
			}
			resolved.EnneagramType = binding
		default:
			resolved.Diagnostics = append(resolved.Diagnostics, Diagnostic{Layer: row.Layer, Code: "unknown_layer"})
		}
	}
	for _, typeNumber := range normalizedTypes {
		if binding := requestedBindings[typeNumber]; binding != nil {
			resolved.RequestedTypeBindings = append(resolved.RequestedTypeBindings, binding)
		}
	}
	return resolved
}

func normalizeRequestedTypes(requestedTypes []int) []int {
	seen := make(map[int]struct{}, len(requestedTypes))
	normalized := make([]int, 0, len(requestedTypes))
	for _, typeNumber := range requestedTypes {
		if typeNumber < 1 || typeNumber > 9 {
			continue
		}
		if _, duplicate := seen[typeNumber]; duplicate {
			continue
		}
		seen[typeNumber] = struct{}{}
		normalized = append(normalized, typeNumber)
	}
	sort.Ints(normalized)
	return normalized
}

func typeTraceKey(typeNumber int) string {
	return fmt.Sprintf("%s_%02d", LayerEnneagramType, typeNumber)
}
