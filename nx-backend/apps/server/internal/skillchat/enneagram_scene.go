package skillchat

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"nine-xing/nx-backend/apps/server/internal/rag"
	"regexp"
	"strings"
)

var sceneTypePattern = regexp.MustCompile(`(?:^我：|^对方：[^，\n]+，)([1-9])\s*号\s*$`)

func sceneKnowledgeKeys(initial string) []string {
	keys := []string{"enneagram-core"}
	seen := map[string]bool{}
	// The app's first four lines contain scene, self and other; never infer a type from event prose.
	for i, line := range strings.Split(initial, "\n") {
		if i > 3 || strings.HasPrefix(line, "具体事件：") {
			break
		}
		match := sceneTypePattern.FindStringSubmatch(strings.TrimSpace(line))
		if len(match) != 2 {
			continue
		}
		key := fmt.Sprintf("enneagram-type-0%s", match[1])
		if !seen[key] {
			keys = append(keys, key)
			seen[key] = true
		}
	}
	return keys
}

// FirstQuestion preserves the scene's form even after old turns have been summarized.
func (s *Store) FirstQuestion(ctx context.Context, appUserID, sessionID int64) (string, error) {
	if err := s.available(); err != nil {
		return "", err
	}
	var question string
	err := s.db.QueryRowContext(ctx, `SELECT message.content FROM app_chat_messages message
 JOIN app_chat_sessions session ON session.id=message.session_id
 WHERE session.id=$1 AND session.app_user_id=$2 AND session.scene='skill_chat' AND message.role='user'
 ORDER BY message.create_time,message.id LIMIT 1`, sessionID, appUserID).Scan(&question)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return question, err
}

func (r *Runtime) sceneKnowledge(ctx context.Context, appUserID, sessionID, releaseID int64, question string) ([]rag.Document, string, error) {
	initialStore, ok := r.store.(interface {
		FirstQuestion(context.Context, int64, int64) (string, error)
	})
	if !ok {
		return nil, "", errors.New("scene context store unavailable")
	}
	initialContext, err := initialStore.FirstQuestion(ctx, appUserID, sessionID)
	if err != nil {
		return nil, "", err
	}
	if initialContext == "" {
		initialContext = question
	}
	search, ok := r.searcher.(interface {
		SearchEnneagramReleaseChunks(context.Context, int64, string, []string, int, float64) ([]rag.Document, error)
	})
	if !ok {
		return nil, "", errors.New("scene knowledge search unavailable")
	}
	documents, err := search.SearchEnneagramReleaseChunks(ctx, releaseID, initialContext+"\n"+question, sceneKnowledgeKeys(initialContext), skillSearchLimit, skillSearchMinScore)
	return documents, initialContext, err
}
