package relationshipinsight

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"
	"time"
)

var (
	ErrNotParticipant = errors.New("insight.not_participant")
	ErrVIPRequired    = errors.New("insight.vip_required")
	ErrNoMessages     = errors.New("insight.insufficient_messages")
	ErrNotFound       = errors.New("insight.not_found")
)

type Metric struct {
	Score      int    `json:"score"`
	Confidence int    `json:"confidence"`
	Trend      string `json:"trend"`
	Evidence   string `json:"evidence"`
}

type Report struct {
	ID                      int64             `json:"id"`
	ConversationID          int64             `json:"conversationId"`
	PeerID                  int64             `json:"peerId"`
	FromSequence            int64             `json:"fromSequence"`
	ToSequence              int64             `json:"toSequence"`
	MessageCount            int               `json:"messageCount"`
	Status                  string            `json:"status"`
	ObservationLevel        string            `json:"observationLevel"`
	PersonalityTypeSnapshot *int              `json:"personalityTypeSnapshot,omitempty"`
	Metrics                 map[string]Metric `json:"metrics"`
	Summary                 string            `json:"summary"`
	PersonalityReference    map[string]string `json:"personalityReference,omitempty"`
	Suggestions             []string          `json:"suggestions"`
	CreatedAt               time.Time         `json:"createdAt"`
}

type messageSample struct {
	SenderID   int64
	Type       string
	Body       string
	SequenceNo int64
	CreatedAt  time.Time
}

type Service struct{ db *sql.DB }

func NewService(db *sql.DB) *Service { return &Service{db: db} }

func (s *Service) Generate(ctx context.Context, initiatorID, conversationID int64) (Report, error) {
	if s == nil || s.db == nil {
		return Report{}, errors.New("relationship insight database is not configured")
	}
	var low, high int64
	if err := s.db.QueryRowContext(ctx, `
		SELECT c.user_low_id,c.user_high_id
		FROM direct_conversations c
		JOIN friendships f ON f.user_low_id=c.user_low_id AND f.user_high_id=c.user_high_id AND f.status='active'
		WHERE c.id=$1 AND c.status='active' AND (c.user_low_id=$2 OR c.user_high_id=$2)`, conversationID, initiatorID).Scan(&low, &high); errors.Is(err, sql.ErrNoRows) {
		return Report{}, ErrNotParticipant
	} else if err != nil {
		return Report{}, err
	}
	if ok, err := s.isVIP(ctx, initiatorID); err != nil {
		return Report{}, err
	} else if !ok {
		return Report{}, ErrVIPRequired
	}
	peerID := low
	if peerID == initiatorID {
		peerID = high
	}
	messages, err := s.loadMessages(ctx, conversationID)
	if err != nil {
		return Report{}, err
	}
	if len(messages) == 0 {
		return Report{}, ErrNoMessages
	}
	report := analyze(initiatorID, peerID, conversationID, messages)
	report.PersonalityTypeSnapshot, report.PersonalityReference, err = s.visiblePersonality(ctx, peerID)
	if err != nil {
		return Report{}, err
	}
	metrics, _ := json.Marshal(report.Metrics)
	reference, _ := json.Marshal(report.PersonalityReference)
	suggestions, _ := json.Marshal(report.Suggestions)
	if report.PersonalityReference == nil {
		reference = nil
	}
	err = s.db.QueryRowContext(ctx, `INSERT INTO relationship_insights(initiator_id,peer_id,conversation_id,from_sequence,to_sequence,message_count,status,observation_level,personality_type_snapshot,metrics,summary,personality_reference,suggestions) VALUES($1,$2,$3,$4,$5,$6,'completed',$7,$8,$9,$10,$11,$12) ON CONFLICT(initiator_id,conversation_id,to_sequence) DO UPDATE SET updated_at=now() RETURNING id,created_at`, initiatorID, peerID, conversationID, report.FromSequence, report.ToSequence, report.MessageCount, report.ObservationLevel, report.PersonalityTypeSnapshot, metrics, report.Summary, reference, suggestions).Scan(&report.ID, &report.CreatedAt)
	return report, err
}

func (s *Service) List(ctx context.Context, initiatorID, conversationID int64) ([]Report, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,conversation_id,peer_id,from_sequence,to_sequence,message_count,status,observation_level,personality_type_snapshot,metrics,summary,personality_reference,suggestions,created_at FROM relationship_insights WHERE initiator_id=$1 AND conversation_id=$2 ORDER BY created_at DESC,id DESC`, initiatorID, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Report{}
	for rows.Next() {
		item, err := scanReport(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return items, nil
	}
	visible, _, err := s.visiblePersonality(ctx, items[0].PeerID)
	if err != nil {
		return nil, err
	}
	if visible == nil {
		for index := range items {
			redactPersonality(&items[index])
		}
	}
	return items, nil
}

func (s *Service) Get(ctx context.Context, initiatorID, id int64) (Report, error) {
	item, err := scanReport(s.db.QueryRowContext(ctx, `SELECT id,conversation_id,peer_id,from_sequence,to_sequence,message_count,status,observation_level,personality_type_snapshot,metrics,summary,personality_reference,suggestions,created_at FROM relationship_insights WHERE id=$1 AND initiator_id=$2`, id, initiatorID))
	if errors.Is(err, sql.ErrNoRows) {
		return Report{}, ErrNotFound
	}
	if err != nil {
		return Report{}, err
	}
	visible, _, err := s.visiblePersonality(ctx, item.PeerID)
	if err != nil {
		return Report{}, err
	}
	if visible == nil {
		redactPersonality(&item)
	}
	return item, nil
}

func redactPersonality(item *Report) {
	item.PersonalityTypeSnapshot = nil
	item.PersonalityReference = nil
}

type scanner interface{ Scan(...any) error }

func scanReport(row scanner) (Report, error) {
	var item Report
	var personality sql.NullInt64
	var metrics, reference, suggestions []byte
	err := row.Scan(&item.ID, &item.ConversationID, &item.PeerID, &item.FromSequence, &item.ToSequence, &item.MessageCount, &item.Status, &item.ObservationLevel, &personality, &metrics, &item.Summary, &reference, &suggestions, &item.CreatedAt)
	if err != nil {
		return Report{}, err
	}
	if personality.Valid {
		value := int(personality.Int64)
		item.PersonalityTypeSnapshot = &value
	}
	_ = json.Unmarshal(metrics, &item.Metrics)
	if len(reference) > 0 {
		_ = json.Unmarshal(reference, &item.PersonalityReference)
	}
	_ = json.Unmarshal(suggestions, &item.Suggestions)
	return item, nil
}

func (s *Service) isVIP(ctx context.Context, userID int64) (bool, error) {
	var level string
	var expires sql.NullTime
	err := s.db.QueryRowContext(ctx, `SELECT member_level,member_expires_at FROM app_users WHERE id=$1 AND status='active'`, userID).Scan(&level, &expires)
	if err != nil {
		return false, err
	}
	return level != "free" && (!expires.Valid || expires.Time.After(time.Now())), nil
}

func (s *Service) loadMessages(ctx context.Context, conversationID int64) ([]messageSample, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT sender_id,message_type,body,sequence_no,created_at FROM direct_messages WHERE conversation_id=$1 AND recalled_at IS NULL ORDER BY sequence_no DESC LIMIT 500`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []messageSample{}
	for rows.Next() {
		var item messageSample
		if err := rows.Scan(&item.SenderID, &item.Type, &item.Body, &item.SequenceNo, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
	return items, rows.Err()
}

func (s *Service) visiblePersonality(ctx context.Context, peerID int64) (*int, map[string]string, error) {
	var visibility string
	var personality sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT u.personality_visibility,c.enneagram FROM app_users u LEFT JOIN LATERAL (SELECT enneagram FROM app_user_cards WHERE app_user_id=u.id AND card_type='primary' AND status='active' ORDER BY update_time DESC,id DESC LIMIT 1) c ON true WHERE u.id=$1`, peerID).Scan(&visibility, &personality)
	if err != nil || visibility != "friends" || !personality.Valid {
		return nil, nil, err
	}
	value := int(personality.Int64)
	return &value, personalityReference(value), nil
}

func analyze(initiatorID, peerID, conversationID int64, messages []messageSample) Report {
	mine, peer, chars, media := 0, 0, 0, 0
	days := map[string]struct{}{}
	positive, tense, support := 0, 0, 0
	positiveWords := []string{"谢谢", "开心", "喜欢", "好的", "哈哈", "爱", "支持"}
	tenseWords := []string{"生气", "烦", "算了", "别说", "失望", "难受", "压力"}
	supportWords := []string{"理解", "陪你", "没关系", "辛苦", "抱抱", "可以帮", "听你"}
	responseMinutes := []float64{}
	for index, message := range messages {
		if message.SenderID == initiatorID {
			mine++
		} else if message.SenderID == peerID {
			peer++
		}
		chars += len([]rune(message.Body))
		if message.Type != "text" {
			media++
		}
		days[message.CreatedAt.Format("2006-01-02")] = struct{}{}
		positive += keywordHits(message.Body, positiveWords)
		tense += keywordHits(message.Body, tenseWords)
		support += keywordHits(message.Body, supportWords)
		if index > 0 && messages[index-1].SenderID != message.SenderID {
			minutes := message.CreatedAt.Sub(messages[index-1].CreatedAt).Minutes()
			if minutes >= 0 && minutes <= 24*60 {
				responseMinutes = append(responseMinutes, minutes)
			}
		}
	}
	total := len(messages)
	confidence := clamp(total*3 + len(days)*8)
	activity := clamp(total*3 + len(days)*7)
	balance := 100
	if total > 0 {
		balance = clamp(100 - int(math.Abs(float64(mine-peer))/float64(total)*100))
	}
	avgMinutes := average(responseMinutes)
	rhythm := clamp(100 - int(avgMinutes/3))
	sentiment := clamp(55 + positive*7 - tense*9)
	supportScore := clamp(35 + support*12)
	boundary := clamp(100 - tense*10)
	temperature := clamp((activity*20 + balance*20 + rhythm*15 + sentiment*25 + supportScore*20) / 100)
	level := "stable"
	if total < 20 || len(days) < 3 {
		level = "preliminary"
	}
	from, to := messages[0].SequenceNo, messages[len(messages)-1].SequenceNo
	metrics := map[string]Metric{
		"activity":    {Score: activity, Confidence: confidence, Trend: "stable", Evidence: evidence(total, len(days), "条消息", "个互动日")},
		"balance":     {Score: balance, Confidence: confidence, Trend: "stable", Evidence: evidence(mine, peer, "次主动表达", "次对方表达")},
		"rhythm":      {Score: rhythm, Confidence: confidence, Trend: "stable", Evidence: "平均回应间隔约 " + roundMinutes(avgMinutes)},
		"sentiment":   {Score: sentiment, Confidence: confidence, Trend: trend(positive, tense), Evidence: evidence(positive, tense, "个积极信号", "个紧张信号")},
		"support":     {Score: supportScore, Confidence: confidence, Trend: "stable", Evidence: evidence(support, media, "个支持表达", "条媒体消息")},
		"boundary":    {Score: boundary, Confidence: confidence, Trend: tenseTrend(tense), Evidence: evidence(tense, total, "个压力信号", "条样本")},
		"temperature": {Score: temperature, Confidence: confidence, Trend: trend(positive+support, tense), Evidence: "综合互动稳定性、互惠性与情绪信号"},
	}
	return Report{ConversationID: conversationID, PeerID: peerID, FromSequence: from, ToSequence: to, MessageCount: total, Status: "completed", ObservationLevel: level, Metrics: metrics, Summary: summary(level, temperature, balance), Suggestions: suggestions(balance, rhythm, tense)}
}

func keywordHits(text string, words []string) int {
	total := 0
	for _, word := range words {
		total += strings.Count(text, word)
	}
	return total
}

func clamp(value int) int {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func average(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sort.Float64s(values)
	return values[len(values)/2]
}

func roundMinutes(value float64) string {
	if value < 1 {
		return "1 分钟内"
	}
	if value >= 60 {
		return strconvItoa(int(math.Round(value/60))) + " 小时"
	}
	return strconvItoa(int(math.Round(value))) + " 分钟"
}

func strconvItoa(value int) string {
	if value == 0 {
		return "0"
	}
	const digits = "0123456789"
	buf := [20]byte{}
	i := len(buf)
	for value > 0 {
		i--
		buf[i] = digits[value%10]
		value /= 10
	}
	return string(buf[i:])
}

func evidence(a, b int, aLabel, bLabel string) string {
	return strconvItoa(a) + aLabel + "，" + strconvItoa(b) + bLabel
}

func trend(positive, negative int) string {
	if positive > negative+1 {
		return "up"
	}
	if negative > positive+1 {
		return "down"
	}
	return "stable"
}

func tenseTrend(value int) string {
	if value > 2 {
		return "attention"
	}
	return "stable"
}

func summary(level string, temperature, balance int) string {
	prefix := "当前为稳定观察"
	if level == "preliminary" {
		prefix = "当前为初步观察，继续聊天后判断会更可靠"
	}
	return prefix + "。关系温度 " + strconvItoa(temperature) + "，主动与回应平衡 " + strconvItoa(balance) + "。这些数值描述当前会话信号，不代表感情结论。"
}

func suggestions(balance, rhythm, tense int) []string {
	items := []string{"在重要话题前先确认对方是否方便交流。", "多用具体感受和需求表达，减少猜测。", "对方回应后先确认理解，再给建议。"}
	if balance < 50 {
		items[0] = "适当为对方留出表达空间，观察互动是否更均衡。"
	}
	if rhythm < 45 {
		items[1] = "把回复间隔视为节奏差异，避免把延迟直接解释为态度。"
	}
	if tense > 2 {
		items[2] = "紧张信号出现时暂停追问，等双方平稳后再继续。"
	}
	return items
}

func personalityReference(personality int) map[string]string {
	profiles := map[int][3]string{
		1: {"重视正确、责任与改进", "具体、尊重原则、肯定努力", "避免只指出不足"},
		2: {"重视被需要与情感连接", "表达感谢，也询问真实需要", "避免把付出视为理所当然"},
		3: {"重视价值、效率与成果", "认可投入，也关心成果之外的感受", "避免只在成功时肯定"},
		4: {"重视独特感受与真实连接", "耐心倾听并允许复杂情绪", "避免过快淡化感受"},
		5: {"重视空间、知识与自主", "提前说明并给予思考时间", "避免连续追问和突然施压"},
		6: {"重视安全、可靠与一致", "给出清晰信息并保持言行一致", "避免含糊承诺"},
		7: {"重视自由、可能性与体验", "用开放选项共同规划", "避免只强调限制"},
		8: {"重视力量、直接与公平", "坦率表达立场并尊重边界", "避免操控或绕弯"},
		9: {"重视和谐、稳定与被看见", "温和邀请表达并给足时间", "避免催促做决定"},
	}
	p := profiles[personality]
	return map[string]string{"label": "基于对方公开的 " + strconvItoa(personality) + " 号主型号理论参考", "coreConcern": p[0], "communicationNeed": p[1], "sensitivePoint": p[2]}
}
