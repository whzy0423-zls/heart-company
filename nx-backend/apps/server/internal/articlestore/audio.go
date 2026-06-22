package articlestore

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// TTSClient turns text into audio bytes. Implemented by the voice package's
// MiniMax client and wired in from the server to avoid a hard import cycle.
type TTSClient interface {
	TextToAudio(ctx context.Context, model string, voiceID string, text string) ([]byte, string, error)
}

// AssetCreator persists generated audio and returns its id. Implemented by the
// uploadasset store.
type AssetCreator interface {
	CreateAudio(ctx context.Context, name string, contentType string, data []byte) (int64, error)
}

// VoiceResolver maps a voice key ("official:xxx" / "clone:<profileId>") to the
// MiniMax voice id to synthesize with. Implemented by the voice store.
type VoiceResolver interface {
	ResolveVoice(ctx context.Context, voiceKey string) (voiceID string, err error)
}

// 单次 TTS 字数上限留余量（MiniMax 限制 5000）。
const ttsChunkRunes = 4500

// 听书音频生成的总时长上限（长文分片串行合成）。
const audioGenTimeout = 8 * time.Minute

// AttachAudioDeps wires the TTS pipeline. Called once from the server.
func (s *Store) AttachAudioDeps(tts TTSClient, assets AssetCreator, voices VoiceResolver, model string) {
	s.tts = tts
	s.assets = assets
	s.voices = voices
	if strings.TrimSpace(model) != "" {
		s.audioModel = model
	}
}

var (
	reCodeBlock  = regexp.MustCompile("(?s)```.*?```")
	reInlineCode = regexp.MustCompile("`[^`]*`")
	reImage      = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	reLink       = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	reHeading    = regexp.MustCompile(`(?m)^#{1,6}\s*`)
	reBlockquote = regexp.MustCompile(`(?m)^>\s?`)
	reListMark   = regexp.MustCompile(`(?m)^\s*([-*+]|\d+\.)\s+`)
	reEmphasis   = regexp.MustCompile(`(\*{1,3}|_{1,3}|~~)`)
	reHr         = regexp.MustCompile(`(?m)^\s*([-*_]\s*){3,}$`)
	reMultiBlank = regexp.MustCompile(`\n{3,}`)
)

// stripMarkdown reduces Markdown to plain speakable text so the TTS engine does
// not read syntax markers aloud.
func stripMarkdown(md string) string {
	text := md
	text = reCodeBlock.ReplaceAllString(text, "")
	text = reImage.ReplaceAllString(text, "")
	text = reInlineCode.ReplaceAllString(text, "$0")
	text = reInlineCode.ReplaceAllStringFunc(text, func(s string) string {
		return strings.Trim(s, "`")
	})
	text = reLink.ReplaceAllString(text, "$1")
	text = reHr.ReplaceAllString(text, "")
	text = reHeading.ReplaceAllString(text, "")
	text = reBlockquote.ReplaceAllString(text, "")
	text = reListMark.ReplaceAllString(text, "")
	text = reEmphasis.ReplaceAllString(text, "")
	text = strings.ReplaceAll(text, "|", " ")
	text = reMultiBlank.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}

// splitForTTS breaks long text into chunks under the per-call rune limit,
// preferring paragraph then sentence boundaries.
func splitForTTS(text string, limit int) []string {
	if limit <= 0 {
		limit = ttsChunkRunes
	}
	var chunks []string
	var buf strings.Builder
	bufRunes := 0

	flush := func() {
		if bufRunes > 0 {
			chunks = append(chunks, strings.TrimSpace(buf.String()))
			buf.Reset()
			bufRunes = 0
		}
	}
	appendPiece := func(piece string) {
		n := len([]rune(piece))
		if bufRunes+n > limit {
			flush()
		}
		buf.WriteString(piece)
		bufRunes += n
	}

	for _, para := range strings.Split(text, "\n") {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		if len([]rune(para)) <= limit {
			appendPiece(para + "\n")
			continue
		}
		// 段落本身超长：按句子切。
		for _, sentence := range splitSentences(para, limit) {
			appendPiece(sentence)
		}
		appendPiece("\n")
	}
	flush()

	out := chunks[:0]
	for _, c := range chunks {
		if strings.TrimSpace(c) != "" {
			out = append(out, c)
		}
	}
	return out
}

// splitSentences chops an over-long paragraph on CJK/ASCII sentence enders,
// hard-cutting any sentence that still exceeds the limit.
func splitSentences(para string, limit int) []string {
	enders := "。！？!?；;\n"
	var sentences []string
	var cur strings.Builder
	for _, r := range para {
		cur.WriteRune(r)
		if strings.ContainsRune(enders, r) && len([]rune(cur.String())) >= limit/2 {
			sentences = append(sentences, cur.String())
			cur.Reset()
		}
	}
	if strings.TrimSpace(cur.String()) != "" {
		sentences = append(sentences, cur.String())
	}

	var out []string
	for _, s := range sentences {
		runes := []rune(s)
		for len(runes) > limit {
			out = append(out, string(runes[:limit]))
			runes = runes[limit:]
		}
		if len(runes) > 0 {
			out = append(out, string(runes))
		}
	}
	return out
}

// readingVoiceKey stores the global default 听书 voice in site_configs.
const readingVoiceKey = "reading_voice"

// DefaultVoice returns the global default voice key (may be empty).
func (s *Store) DefaultVoice(ctx context.Context) (string, error) {
	if s == nil || s.db == nil {
		return "", nil
	}
	c, cancel := s.ctx(ctx)
	defer cancel()
	var raw string
	err := s.db.QueryRowContext(c, `SELECT config->>'voiceKey' FROM site_configs WHERE key=$1`, readingVoiceKey).Scan(&raw)
	if err != nil {
		return "", nil // 未配置时返回空，由调用方决定回退
	}
	return strings.TrimSpace(raw), nil
}

// SetDefaultVoice upserts the global default 听书 voice key.
func (s *Store) SetDefaultVoice(ctx context.Context, voiceKey string) error {
	if s == nil || s.db == nil {
		return errors.New("database is not configured")
	}
	c, cancel := s.ctx(ctx)
	defer cancel()
	payload := fmt.Sprintf(`{"voiceKey":%q}`, strings.TrimSpace(voiceKey))
	_, err := s.db.ExecContext(c,
		`INSERT INTO site_configs (key, config, update_time)
		 VALUES ($1, $2::jsonb, now())
		 ON CONFLICT (key) DO UPDATE SET config=$2::jsonb, update_time=now()`,
		readingVoiceKey, payload,
	)
	return err
}

// GenerateAudio (re)synthesizes the listen-to-article audio for one article and
// caches it on the row. Resolves per-article voice, falling back to the global
// default. Long articles are chunked and the MP3 bytes concatenated.
func (s *Store) GenerateAudio(ctx context.Context, id string) (Article, error) {
	if s == nil || s.db == nil {
		return Article{}, errors.New("database is not configured")
	}
	if s.tts == nil || s.assets == nil || s.voices == nil {
		return Article{}, errors.New("语音服务未配置，无法生成听书音频")
	}

	doc, ok, err := s.GetArticle(ctx, id)
	if err != nil {
		return Article{}, err
	}
	if !ok {
		return Article{}, errors.New("文章不存在")
	}

	voiceKey := strings.TrimSpace(doc.VoiceKey)
	if voiceKey == "" {
		if def, _ := s.DefaultVoice(ctx); def != "" {
			voiceKey = def
		}
	}
	if voiceKey == "" {
		return Article{}, errors.New("请先为文章选择听书音色，或在阅读管理中设置全局默认音色")
	}

	text := stripMarkdown(doc.Content)
	if strings.TrimSpace(text) == "" {
		return Article{}, errors.New("正文为空，无法生成音频")
	}

	gctx, cancel := context.WithTimeout(context.Background(), audioGenTimeout)
	defer cancel()

	voiceID, err := s.voices.ResolveVoice(gctx, voiceKey)
	if err != nil {
		s.markAudioFailed(gctx, id, err.Error())
		return Article{}, err
	}

	s.markAudioStatus(gctx, id, "generating", voiceKey)

	chunks := splitForTTS(text, ttsChunkRunes)
	var combined []byte
	contentType := "audio/mpeg"
	for i, chunk := range chunks {
		audio, ct, err := s.tts.TextToAudio(gctx, s.audioModel, voiceID, chunk)
		if err != nil {
			msg := fmt.Sprintf("第 %d/%d 段合成失败：%s", i+1, len(chunks), err.Error())
			s.markAudioFailed(gctx, id, msg)
			return Article{}, errors.New(msg)
		}
		if ct != "" {
			contentType = ct
		}
		combined = append(combined, audio...)
	}
	if len(combined) == 0 {
		s.markAudioFailed(gctx, id, "未生成任何音频数据")
		return Article{}, errors.New("未生成任何音频数据")
	}

	assetID, err := s.assets.CreateAudio(gctx,
		fmt.Sprintf("article-%s-%s.mp3", id, time.Now().Format("20060102150405")),
		contentType, combined)
	if err != nil {
		s.markAudioFailed(gctx, id, err.Error())
		return Article{}, err
	}
	audioURL := fmt.Sprintf("/api/upload-assets/%d", assetID)

	c, cancelC := s.ctx(ctx)
	defer cancelC()
	if _, err := s.db.ExecContext(c,
		`UPDATE articles
		    SET audio_asset_id=$1, audio_url=$2, audio_voice_key=$3,
		        audio_status='ready', audio_error='', audio_time=now()
		  WHERE id=$4`,
		assetID, audioURL, voiceKey, id,
	); err != nil {
		return Article{}, err
	}

	updated, _, err := s.GetArticle(ctx, id)
	return updated, err
}

func (s *Store) markAudioStatus(ctx context.Context, id, status, voiceKey string) {
	c, cancel := s.ctx(ctx)
	defer cancel()
	_, _ = s.db.ExecContext(c,
		`UPDATE articles SET audio_status=$1, audio_error='', audio_voice_key=$2 WHERE id=$3`,
		status, voiceKey, id)
}

func (s *Store) markAudioFailed(ctx context.Context, id, msg string) {
	c, cancel := s.ctx(ctx)
	defer cancel()
	_, _ = s.db.ExecContext(c,
		`UPDATE articles SET audio_status='failed', audio_error=$1 WHERE id=$2`,
		truncateRunes(msg, 500), id)
}
