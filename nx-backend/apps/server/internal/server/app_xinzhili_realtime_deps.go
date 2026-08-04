package server

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/chat"
	"nine-xing/nx-backend/apps/server/internal/rag"
	"nine-xing/nx-backend/apps/server/internal/theorystore"
	"nine-xing/nx-backend/apps/server/internal/xinzhili"
)

func (s *Server) newXinzhiliRealtimeDependencies(cfg xinzhili.Config, sink xinzhili.SessionSink) (xinzhili.SessionDependencies, error) {
	chatStore, ok := s.appChat.(*chat.Store)
	if !ok || chatStore == nil {
		return xinzhili.SessionDependencies{}, errors.New("xinzhili chat store unavailable")
	}
	provider := (xinzhili.TTSProviderFactory{}).Dynamic()
	var generator xinzhili.ChatGenerator
	if current := s.generator(); current != nil {
		generator = serverXinzhiliGenerator{generator: current}
	}
	return xinzhili.SessionDependencies{
		Cards:         serverXinzhiliCards{server: s},
		Conversations: serverXinzhiliConversations{store: chatStore},
		Preferences:   serverXinzhiliPreferences{server: s},
		Memories:      serverXinzhiliMemories{server: s},
		Knowledge:     serverXinzhiliKnowledge{server: s},
		Theory:        serverXinzhiliTheory{store: theorystore.NewStore(s.db)},
		Generator:     generator,
		ASRFactory:    xinzhili.NewAliyunASRFactory(xinzhili.AliyunASROptions{}),
		Synthesizer:   xinzhili.NewSynthesizer(provider, 1, xinzhili.WithTTSSegmentTimeout(45*time.Second), xinzhili.WithSingleSegmentTTSInput()),
		EngineFactory: func(mode xinzhili.Mode, timing xinzhili.TimingConfig, clock xinzhili.Clock) xinzhili.StrategyEngine {
			return xinzhili.NewEngine(mode, timing, clock)
		},
		Sink:  sink,
		Clock: serverXinzhiliClock{},
	}, nil
}

type serverXinzhiliClock struct{}

func (serverXinzhiliClock) Now() time.Time { return time.Now() }

type serverXinzhiliCards struct{ server *Server }

func (a serverXinzhiliCards) OwnedCard(ctx context.Context, userID, cardID int64) (xinzhili.Card, error) {
	if a.server == nil || a.server.quiz == nil {
		return xinzhili.Card{}, errors.New("card store unavailable")
	}
	card, err := a.server.quiz.GetCard(ctx, userID, cardID)
	if err != nil {
		return xinzhili.Card{}, err
	}
	return xinzhili.Card{
		ID: card.ID, Name: card.Name, Relation: card.Relation, MainType: card.MainType,
		WingType: card.WingType, Profile: strings.TrimSpace(string(card.Profile)),
	}, nil
}

type serverXinzhiliConversations struct{ store *chat.Store }

func (a serverXinzhiliConversations) Resolve(ctx context.Context, userID, cardID int64, scene string, conversationID int64) (xinzhili.Conversation, error) {
	session, err := a.store.ResolveSceneSession(ctx, userID, cardID, scene, conversationID)
	if err != nil {
		return xinzhili.Conversation{}, err
	}
	return xinzhili.Conversation{ID: session.ID, UserID: session.AppUserID, CardID: session.CardID, Scene: scene}, nil
}

func (a serverXinzhiliConversations) History(ctx context.Context, conversation xinzhili.Conversation, limit int) ([]rag.Message, string, error) {
	state, err := a.store.GetConversationState(ctx, conversation.ID)
	if err != nil {
		return nil, "", err
	}
	messages, err := a.store.ListRecentMessages(ctx, conversation.ID, limit)
	if err != nil {
		return nil, "", err
	}
	history := make([]rag.Message, 0, len(messages))
	for _, message := range messages {
		content := message.EffectiveContent()
		if content == "" {
			continue
		}
		history = append(history, rag.Message{Role: message.Role, Content: content})
	}
	return history, strings.TrimSpace(state.Summary), nil
}

func (a serverXinzhiliConversations) SaveUser(ctx context.Context, conversation xinzhili.Conversation, text string, mode xinzhili.Mode) (int64, error) {
	return a.store.SaveSceneUserText(ctx, conversation.ID, text, string(mode))
}

func (a serverXinzhiliConversations) CreateAssistant(ctx context.Context, conversation xinzhili.Conversation, content string, mode xinzhili.Mode) (int64, error) {
	return a.store.CreateSceneAssistant(ctx, conversation.ID, content, string(mode))
}

func (a serverXinzhiliConversations) AcknowledgeAssistant(ctx context.Context, messageID int64, deliveredText string, complete bool) error {
	return a.store.AcknowledgeSceneAssistant(ctx, messageID, deliveredText, complete)
}

func (a serverXinzhiliConversations) CompleteAssistant(ctx context.Context, messageID int64, content string, sources json.RawMessage) error {
	return a.store.CompleteSceneAssistant(ctx, messageID, content, sources)
}

type serverXinzhiliPreferences struct{ server *Server }

func (a serverXinzhiliPreferences) PromptPreferences(ctx context.Context, userID int64) ([]string, error) {
	if a.server == nil || a.server.userPreferences == nil {
		return nil, nil
	}
	preferences, err := a.server.userPreferences.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(preferences))
	for _, preference := range preferences {
		if instruction := strings.TrimSpace(preference.Instruction); instruction != "" {
			result = append(result, instruction)
		}
	}
	return result, nil
}

type serverXinzhiliMemories struct{ server *Server }

func (a serverXinzhiliMemories) PromptMemories(ctx context.Context, userID, cardID int64) ([]string, error) {
	if a.server == nil || a.server.db == nil {
		return nil, nil
	}
	return a.server.appChatMemoriesForPrompt(ctx, userID, cardID, 6)
}

type serverXinzhiliKnowledge struct{ server *Server }

func (a serverXinzhiliKnowledge) Search(ctx context.Context, query string, topK int, minScore float64) ([]rag.Document, error) {
	if a.server == nil {
		return nil, nil
	}
	documents, err := a.server.retrieveAppDocsForQuery(ctx, query, topK*2)
	if err != nil {
		return nil, err
	}
	return filterXinzhiliKnowledge(query, documents, topK, minScore), nil
}

type serverXinzhiliTheory struct{ store *theorystore.Store }

func (a serverXinzhiliTheory) Search(ctx context.Context, query string, topK int, minScore float64) ([]rag.Document, error) {
	return a.store.SearchActiveChunks(ctx, query, topK, minScore)
}

type serverXinzhiliGenerator struct{ generator rag.Generator }

func (a serverXinzhiliGenerator) GenerateStream(ctx context.Context, input rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
	if streaming, ok := a.generator.(rag.StreamingGenerator); ok {
		return streaming.GenerateStream(ctx, input, emit)
	}
	answer, err := a.generator.Generate(ctx, input)
	if err == nil && emit != nil && answer != "" {
		if emitErr := emit(answer); emitErr != nil {
			return "", emitErr
		}
	}
	return answer, err
}

func filterXinzhiliKnowledge(query string, documents []rag.Document, topK int, minScore float64) []rag.Document {
	if topK <= 0 {
		return nil
	}
	queryGrams := xinzhiliSearchBigrams(query)
	if len(queryGrams) == 0 {
		return nil
	}
	if minScore <= 0 {
		minScore = 0.2
	}
	result := make([]rag.Document, 0, topK)
	for _, document := range documents {
		candidate := xinzhiliSearchBigrams(document.Title + document.Content + strings.Join(document.Tags, ""))
		matches := 0
		for gram := range queryGrams {
			if _, ok := candidate[gram]; ok {
				matches++
			}
		}
		score := float64(matches) / float64(len(queryGrams))
		if score < minScore {
			continue
		}
		result = append(result, document)
		if len(result) == topK {
			break
		}
	}
	return result
}

func xinzhiliSearchBigrams(value string) map[string]struct{} {
	runes := []rune(strings.ToLower(strings.Join(strings.Fields(value), "")))
	result := make(map[string]struct{})
	for i := 0; i+1 < len(runes); i++ {
		result[string(runes[i:i+2])] = struct{}{}
	}
	return result
}
