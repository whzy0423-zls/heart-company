package lifestory

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// GenerationPayloadHash returns a deterministic digest for the complete
// client generation request. The request key is deliberately included: a
// repeated key with a different payload is a conflict, never a second job.
func GenerationPayloadHash(input GenerationInput) (string, error) {
	input.RequestKey = strings.TrimSpace(input.RequestKey)
	input.Instruction = strings.TrimSpace(input.Instruction)
	if input.RequestKey == "" {
		return "", fmt.Errorf("request key is required")
	}
	if input.FactsVersion < 0 || input.OutlineVersion < 0 || input.SourceVersionID < 0 {
		return "", fmt.Errorf("generation versions must be non-negative")
	}
	if len([]rune(input.Instruction)) > 6000 {
		return "", fmt.Errorf("generation instruction is too long")
	}
	raw, err := json.Marshal(struct {
		RequestKey      string `json:"requestKey"`
		FactsVersion    int64  `json:"factsVersion"`
		OutlineVersion  int64  `json:"outlineVersion"`
		SourceVersionID int64  `json:"sourceVersionId,omitempty"`
		Instruction     string `json:"instruction,omitempty"`
	}{
		RequestKey: input.RequestKey, FactsVersion: input.FactsVersion,
		OutlineVersion: input.OutlineVersion, SourceVersionID: input.SourceVersionID,
		Instruction: input.Instruction,
	})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func snapshotPayloadHash(raw []byte) string {
	raw = canonicalSnapshotJSON(raw)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func snapshotPayloadHMAC(raw, key []byte) string {
	raw = canonicalSnapshotJSON(raw)
	if len(key) == 0 {
		key = tokenKeyBytes("")
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("life-story-generation-payload-v1\x00"))
	_, _ = mac.Write(raw)
	return hex.EncodeToString(mac.Sum(nil))
}

func canonicalSnapshotJSON(raw []byte) []byte {
	// PostgreSQL JSONB normalizes whitespace and key order. Re-marshal through
	// encoding/json so digests are stable before insertion and after retrieval.
	var value any
	if err := json.Unmarshal(raw, &value); err == nil {
		if canonical, marshalErr := json.Marshal(value); marshalErr == nil {
			return canonical
		}
	}
	return raw
}

// TokenReplacement is kept in memory by the worker. Value is never placed in
// an LLM prompt; only the opaque token is persisted in the input snapshot.
type TokenReplacement struct {
	Token   string `json:"token"`
	Value   string `json:"value"`
	Kind    string `json:"kind"`
	Output  string `json:"output,omitempty"`
	Allowed bool   `json:"allowed"`
}

type TokenMap map[string]TokenReplacement

func tokenKeyBytes(secret string) []byte {
	if strings.TrimSpace(secret) == "" {
		secret = "life-story-local-token-key"
	}
	digest := sha256.Sum256([]byte(secret))
	return digest[:]
}

func encryptTokenMap(tokens TokenMap, key []byte) ([]byte, error) {
	if len(tokens) == 0 {
		return nil, nil
	}
	if len(key) != 32 {
		key = tokenKeyBytes(string(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plain, err := json.Marshal(tokens)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plain, nil), nil
}

func decryptTokenMap(ciphertext, key []byte) (TokenMap, error) {
	if len(ciphertext) == 0 {
		return TokenMap{}, nil
	}
	if len(key) != 32 {
		key = tokenKeyBytes(string(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < gcm.NonceSize() {
		return nil, fmt.Errorf("invalid encrypted token map")
	}
	nonce, sealed := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt token map: %w", err)
	}
	var tokens TokenMap
	if err := json.Unmarshal(plain, &tokens); err != nil {
		return nil, fmt.Errorf("decode token map: %w", err)
	}
	if tokens == nil {
		tokens = TokenMap{}
	}
	return tokens, nil
}

var (
	tokenWordPattern = regexp.MustCompile(`\{\{[A-Z]+_[0-9]+\}\}`)
	piiPatterns      = []struct {
		kind    string
		pattern *regexp.Regexp
	}{
		{kind: "email", pattern: regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`)},
		{kind: "phone", pattern: regexp.MustCompile(`\b1[3-9][0-9]{9}\b`)},
		{kind: "phone", pattern: regexp.MustCompile(`\b0[0-9]{2,3}-?[0-9]{7,8}\b`)},
		{kind: "id", pattern: regexp.MustCompile(`\b[1-9][0-9]{5}(?:19|20)[0-9]{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12][0-9]|3[01])[0-9]{3}[0-9Xx]\b`)},
		{kind: "org", pattern: regexp.MustCompile(`[\p{Han}A-Za-z0-9]{2,16}(?:大学|学院|学校|中学|小学|公司|集团|工厂|研究所)`)},
		{kind: "address", pattern: regexp.MustCompile(`[\p{Han}]{2,16}(?:省|市|区|县|镇|乡|街道|路|街|巷)[\p{Han}A-Za-z0-9\-]{0,16}(?:号|栋|单元|室)?`)},
	}
	directPIIPatterns = []*regexp.Regexp{
		piiPatterns[0].pattern,
		piiPatterns[1].pattern,
		piiPatterns[2].pattern,
		piiPatterns[3].pattern,
	}
)

// TokenizeSnapshot makes the model boundary explicit. It deep-copies the
// snapshot, replaces names and non-public locations in all free text, and
// removes raw real-name fields from the model-visible fact card.
func TokenizeSnapshot(snapshot StorySnapshot) (StorySnapshot, TokenMap) {
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return snapshot, nil
	}
	var safe StorySnapshot
	if json.Unmarshal(raw, &safe) != nil {
		return snapshot, nil
	}
	tokens := TokenMap{}
	tokenOrder := make([]string, 0)
	counters := make(map[string]int)
	addToken := func(value, kind string, allowed bool, outputValues ...string) string {
		value = strings.TrimSpace(value)
		if value == "" {
			return ""
		}
		for token, replacement := range tokens {
			if replacement.Value == value {
				if replacement.Allowed && !allowed {
					output := ""
					if len(outputValues) > 0 {
						output = strings.TrimSpace(outputValues[0])
					}
					if output == "" || output == value {
						output = safeTokenOutput(kind)
					}
					replacement.Allowed = false
					replacement.Output = output
					tokens[token] = replacement
				}
				return token
			}
		}
		prefix := strings.ToUpper(strings.TrimSpace(kind))
		if prefix == "" {
			prefix = "PRIVATE"
		}
		counters[prefix]++
		ordinal := counters[prefix]
		token := fmt.Sprintf("{{%s_%d}}", prefix, ordinal)
		output := ""
		if len(outputValues) > 0 {
			output = strings.TrimSpace(outputValues[0])
		}
		if allowed {
			output = value
		} else if output == "" || output == value {
			output = safeTokenOutput(kind)
		}
		tokens[token] = TokenReplacement{Token: token, Value: value, Kind: kind, Output: output, Allowed: allowed}
		tokenOrder = append(tokenOrder, token)
		return token
	}
	replace := func(value string) string {
		for _, token := range tokenOrder {
			replacement := tokens[token]
			value = strings.ReplaceAll(value, replacement.Value, token)
		}
		return value
	}
	for i := range safe.FactCard.Characters {
		character := &safe.FactCard.Characters[i]
		realName := strings.TrimSpace(character.RealName)
		allowed := privacyModesAllowReal(character.PrivacyMode, character.RedactionMode)
		pseudonym := privacyModesUsePseudonym(character.PrivacyMode, character.RedactionMode)
		if realName != "" {
			safeName := ""
			if pseudonym {
				safeName = strings.TrimSpace(character.Alias)
				if safeName == "" && strings.TrimSpace(character.Name) != realName {
					safeName = strings.TrimSpace(character.Name)
				}
			}
			addToken(realName, "person", allowed, safeName)
			character.RealName = ""
			if strings.TrimSpace(character.Alias) == "" {
				character.Alias = character.Name
			}
		}
		name := strings.TrimSpace(character.Name)
		alias := strings.TrimSpace(character.Alias)
		if name != "" && !allowed {
			if pseudonym {
				if name != alias {
					addToken(name, "person", false, alias)
					if alias != "" {
						character.Name = alias
					}
				}
			} else {
				addToken(name, "person", false)
			}
		}
		if alias != "" && alias != name && !allowed && !pseudonym {
			addToken(alias, "person", false)
		}
	}
	for i := range safe.FactCard.Organizations {
		organization := &safe.FactCard.Organizations[i]
		name := strings.TrimSpace(organization.Name)
		if name == "" {
			continue
		}
		allowed := privacyModesAllowReal(organization.RedactionMode)
		pseudonym := privacyModesUsePseudonym(organization.RedactionMode)
		alias := strings.TrimSpace(organization.Alias)
		safeName := ""
		if pseudonym && alias != name {
			safeName = alias
		}
		organization.Name = addToken(name, "org", allowed, safeName)
		if !pseudonym || alias == name {
			organization.Alias = ""
		}
	}
	for i := range safe.FactCard.Events {
		event := &safe.FactCard.Events[i]
		location := strings.TrimSpace(event.Location)
		if location != "" {
			addToken(location, "place", privacyModesAllowReal(event.RedactionMode))
			event.Location = ""
		}
	}
	for i := range safe.FactCard.Timeline {
		event := &safe.FactCard.Timeline[i]
		location := strings.TrimSpace(event.Location)
		if location != "" {
			addToken(location, "place", privacyModesAllowReal(event.RedactionMode))
			event.Location = ""
		}
	}
	redact := func(value string) string {
		value = replace(value)
		for _, rule := range piiPatterns {
			matches := rule.pattern.FindAllString(value, -1)
			for _, match := range matches {
				if match == "" || tokenWordPattern.MatchString(match) {
					continue
				}
				token := addToken(match, rule.kind, false)
				value = strings.ReplaceAll(value, match, token)
			}
		}
		return value
	}
	for i := range safe.Materials {
		safe.Materials[i].Text = redact(safe.Materials[i].Text)
		safe.Materials[i].Transcript = redact(safe.Materials[i].Transcript)
	}
	for i := range safe.FactCard.Characters {
		safe.FactCard.Characters[i].Name = redact(safe.FactCard.Characters[i].Name)
		safe.FactCard.Characters[i].Alias = redact(safe.FactCard.Characters[i].Alias)
		safe.FactCard.Characters[i].Relation = redact(safe.FactCard.Characters[i].Relation)
		safe.FactCard.Characters[i].Role = redact(safe.FactCard.Characters[i].Role)
		safe.FactCard.Characters[i].Description = redact(safe.FactCard.Characters[i].Description)
	}
	for i := range safe.FactCard.Events {
		safe.FactCard.Events[i].Description = redact(safe.FactCard.Events[i].Description)
		safe.FactCard.Events[i].Time = redact(safe.FactCard.Events[i].Time)
		safe.FactCard.Events[i].TurningPoint = redact(safe.FactCard.Events[i].TurningPoint)
		safe.FactCard.Events[i].Outcome = redact(safe.FactCard.Events[i].Outcome)
		for j := range safe.FactCard.Events[i].People {
			safe.FactCard.Events[i].People[j] = redact(safe.FactCard.Events[i].People[j])
		}
	}
	for i := range safe.FactCard.Timeline {
		safe.FactCard.Timeline[i].Description = redact(safe.FactCard.Timeline[i].Description)
		safe.FactCard.Timeline[i].Time = redact(safe.FactCard.Timeline[i].Time)
		safe.FactCard.Timeline[i].TurningPoint = redact(safe.FactCard.Timeline[i].TurningPoint)
		safe.FactCard.Timeline[i].Outcome = redact(safe.FactCard.Timeline[i].Outcome)
		for j := range safe.FactCard.Timeline[i].People {
			safe.FactCard.Timeline[i].People[j] = redact(safe.FactCard.Timeline[i].People[j])
		}
	}
	for i := range safe.FactCard.Questions {
		safe.FactCard.Questions[i].Prompt = redact(safe.FactCard.Questions[i].Prompt)
		safe.FactCard.Questions[i].Answer = redact(safe.FactCard.Questions[i].Answer)
	}
	for i := range safe.Outline.Chapters {
		safe.Outline.Chapters[i].Title = redact(safe.Outline.Chapters[i].Title)
		safe.Outline.Chapters[i].Summary = redact(safe.Outline.Chapters[i].Summary)
		safe.Outline.Chapters[i].Beat = redact(safe.Outline.Chapters[i].Beat)
		for j := range safe.Outline.Chapters[i].KeyBeats {
			safe.Outline.Chapters[i].KeyBeats[j] = redact(safe.Outline.Chapters[i].KeyBeats[j])
		}
	}
	safe.FactCard.Setting = redact(safe.FactCard.Setting)
	safe.FactCard.Conflict = redact(safe.FactCard.Conflict)
	safe.FactCard.TurningPoint = redact(safe.FactCard.TurningPoint)
	safe.FactCard.CentralQuestion = redact(safe.FactCard.CentralQuestion)
	safe.FactCard.Ending = redact(safe.FactCard.Ending)
	safe.FactCard.Unresolved = redact(safe.FactCard.Unresolved)
	safe.RevisionInstruction = redact(safe.RevisionInstruction)
	return safe, tokens
}

func privacyModesAllowReal(modes ...string) bool {
	seen := false
	for _, mode := range modes {
		if mode == "" {
			continue
		}
		if mode != "real" {
			return false
		}
		seen = true
	}
	return seen
}

func privacyModesUsePseudonym(modes ...string) bool {
	seenPseudonym := false
	for _, mode := range modes {
		if mode == "" {
			continue
		}
		switch mode {
		case "pseudonym":
			seenPseudonym = true
		case "real":
			// A conflicting real request still fails closed to the safe alias.
		default:
			return false
		}
	}
	return seenPseudonym
}

func safeTokenOutput(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "person":
		return "某人"
	case "place", "address":
		return "某地"
	case "org":
		return "某机构"
	case "phone", "email":
		return "隐去的联系方式"
	case "id":
		return "隐去的身份信息"
	default:
		return "隐去的信息"
	}
}

func restoreSafeTokens(version Version, tokens TokenMap) Version {
	restore := func(value string) string {
		for token, replacement := range tokens {
			output := strings.TrimSpace(replacement.Output)
			if replacement.Allowed {
				output = replacement.Value
			} else if output == "" {
				output = safeTokenOutput(replacement.Kind)
			}
			value = strings.ReplaceAll(value, token, output)
		}
		return value
	}
	for i := range version.Chapters {
		version.Chapters[i].Title = restore(version.Chapters[i].Title)
		version.Chapters[i].Summary = restore(version.Chapters[i].Summary)
		version.Chapters[i].Body = restore(version.Chapters[i].Body)
	}
	version.Reflection = restore(version.Reflection)
	return version
}

// ContainsSensitiveToken detects both opaque tokens and values that should
// never appear in a model response. It is intentionally conservative.
func ContainsSensitiveToken(version Version, snapshot StorySnapshot, tokens TokenMap) bool {
	text := versionText(version)
	if tokenWordPattern.MatchString(text) {
		return true
	}
	for _, pattern := range directPIIPatterns {
		if pattern.MatchString(text) {
			return true
		}
	}
	for token, replacement := range tokens {
		if strings.Contains(text, token) {
			return true
		}
		if !replacement.Allowed && replacement.Value != "" && strings.Contains(text, replacement.Value) {
			return true
		}
	}
	for _, character := range snapshot.FactCard.Characters {
		if real := strings.TrimSpace(character.RealName); real != "" && strings.TrimSpace(character.Alias) != real && strings.Contains(text, real) {
			return true
		}
	}
	// Worker snapshots are already tokenized before persistence. Their encrypted
	// map stays outside the model input, so the persisted placeholders themselves
	// are also treated as sensitive output even when this call has no fresh map.
	if raw, err := json.Marshal(snapshot); err == nil {
		for _, token := range tokenWordPattern.FindAllString(string(raw), -1) {
			if strings.Contains(text, token) {
				return true
			}
		}
	}
	return false
}

func versionText(version Version) string {
	var builder strings.Builder
	for _, chapter := range version.Chapters {
		builder.WriteString(chapter.Title)
		builder.WriteByte('\n')
		builder.WriteString(chapter.Summary)
		builder.WriteByte('\n')
		builder.WriteString(chapter.Body)
		builder.WriteByte('\n')
	}
	builder.WriteString(version.Reflection)
	return builder.String()
}

func hasOpaqueToken(value string) bool {
	return tokenWordPattern.MatchString(value)
}
