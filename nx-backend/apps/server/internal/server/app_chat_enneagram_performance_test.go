package server

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	_ "github.com/jackc/pgx/v5/stdlib"
)

var (
	mainChatPerformanceMode         = flag.String("mode", "", "live performance mode: baseline or candidate")
	mainChatPerformanceBaseURL      = flag.String("base-url", "", "base URL for the live backend")
	mainChatPerformanceTokenEnv     = flag.String("token-env", "", "environment variable containing the app bearer token")
	mainChatPerformanceSessionID    = flag.Int64("session-id", 0, "dedicated performance fixture chat session")
	mainChatPerformanceDatabaseEnv  = flag.String("database-url-env", "", "environment variable containing the test database URL")
	mainChatPerformanceBaselineFile = flag.String("baseline-file", "", "baseline JSONL file for candidate comparison")
	mainChatPerformanceSamples      = flag.Int("samples", 20, "measured samples per case")
	mainChatPerformanceWarmups      = flag.Int("warmups", 2, "warm-up requests per case")
	mainChatPerformanceRequestGap   = flag.Duration("min-request-interval", 6*time.Second, "minimum interval between request starts")
	mainChatPerformanceOutput       = flag.String("output", "", "JSONL metrics output path")
)

type mainChatPerformanceMetric struct {
	Timestamp             time.Time `json:"timestamp"`
	Status                int       `json:"status"`
	TTFTMillis            int64     `json:"ttft_ms"`
	CompletionMillis      int64     `json:"completion_ms"`
	ResponseRunes         int       `json:"response_runes"`
	RequestedTypeCount    int       `json:"requested_type_count"`
	CoveredDimensionCount int       `json:"covered_dimension_count"`
	Truncated             bool      `json:"truncated"`
}

type mainChatPerformanceCase struct {
	question string
	types    []int
	stream   bool
}

func TestMainChatEnneagramPerformanceGate(t *testing.T) {
	if strings.TrimSpace(*mainChatPerformanceMode) == "" && strings.TrimSpace(*mainChatPerformanceBaseURL) == "" {
		t.Skip("live main-chat performance flags not supplied")
	}
	mode := strings.ToLower(strings.TrimSpace(*mainChatPerformanceMode))
	if mode != "baseline" && mode != "candidate" {
		t.Fatal("-mode must be baseline or candidate")
	}
	if strings.TrimSpace(*mainChatPerformanceBaseURL) == "" || strings.TrimSpace(*mainChatPerformanceTokenEnv) == "" ||
		*mainChatPerformanceSessionID <= 0 || strings.TrimSpace(*mainChatPerformanceDatabaseEnv) == "" || strings.TrimSpace(*mainChatPerformanceOutput) == "" {
		t.Fatal("-base-url, -token-env, -session-id, -database-url-env, and -output are required")
	}
	if *mainChatPerformanceSamples <= 0 || *mainChatPerformanceWarmups < 0 || *mainChatPerformanceRequestGap < 0 {
		t.Fatal("-samples must be positive and warmups/interval must be non-negative")
	}
	if mode == "candidate" && strings.TrimSpace(*mainChatPerformanceBaselineFile) == "" {
		t.Fatal("-baseline-file is required in candidate mode")
	}
	if mode == "candidate" && filepath.Clean(*mainChatPerformanceBaselineFile) == filepath.Clean(*mainChatPerformanceOutput) {
		t.Fatal("-baseline-file and -output must be different files")
	}

	token := strings.TrimSpace(os.Getenv(*mainChatPerformanceTokenEnv))
	databaseURL := strings.TrimSpace(os.Getenv(*mainChatPerformanceDatabaseEnv))
	if token == "" || databaseURL == "" {
		t.Fatal("token or database URL environment variable is empty")
	}
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := database.PingContext(ctx); err != nil {
		t.Fatalf("performance database unavailable: %v", err)
	}
	if err := verifyMainChatPerformanceFixture(ctx, database, *mainChatPerformanceSessionID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		if err := resetMainChatPerformanceFixture(cleanupCtx, database, *mainChatPerformanceSessionID); err != nil {
			t.Errorf("final performance fixture reset: %v", err)
		}
	})

	if err := os.MkdirAll(filepath.Dir(*mainChatPerformanceOutput), 0o755); err != nil {
		t.Fatal(err)
	}
	output, err := os.Create(*mainChatPerformanceOutput)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = output.Close() })
	encoder := json.NewEncoder(output)

	cases := mainChatPerformanceCases(mode)
	client := &http.Client{Timeout: 90 * time.Second}
	var lastRequestStart time.Time
	metrics := make([]mainChatPerformanceMetric, 0, len(cases)**mainChatPerformanceSamples)
	for _, testCase := range cases {
		for index := -*mainChatPerformanceWarmups; index < *mainChatPerformanceSamples; index++ {
			resetCtx, resetCancel := context.WithTimeout(context.Background(), 15*time.Second)
			err := resetMainChatPerformanceFixture(resetCtx, database, *mainChatPerformanceSessionID)
			resetCancel()
			if err != nil {
				t.Fatalf("reset fixture before request: %v", err)
			}
			if wait := *mainChatPerformanceRequestGap - time.Since(lastRequestStart); !lastRequestStart.IsZero() && wait > 0 {
				time.Sleep(wait)
			}
			lastRequestStart = time.Now()
			metric, answer, err := measureMainChatPerformanceRequest(client, strings.TrimRight(*mainChatPerformanceBaseURL, "/"), token, *mainChatPerformanceSessionID, testCase)
			if err != nil {
				t.Fatalf("live request failed: %v", err)
			}
			if mode == "candidate" {
				validateMainChatPerformanceAnswer(t, testCase, metric, answer)
			}
			answer = ""
			if index < 0 {
				continue
			}
			metrics = append(metrics, metric)
			if err := encoder.Encode(metric); err != nil {
				t.Fatal(err)
			}
		}
	}
	if mode == "candidate" {
		applyMainChatPerformanceGates(t, metrics, readMainChatPerformanceMetrics(t, *mainChatPerformanceBaselineFile))
	}
}

func mainChatPerformanceCases(mode string) []mainChatPerformanceCase {
	if mode == "baseline" {
		return []mainChatPerformanceCase{
			{question: "今天适合散步吗？"},
			{question: "今天适合散步吗？", stream: true},
			{question: "什么是九型人格", types: []int{1, 2, 3, 4, 5, 6, 7, 8, 9}},
			{question: "什么是九型人格", types: []int{1, 2, 3, 4, 5, 6, 7, 8, 9}, stream: true},
		}
	}
	cases := []mainChatPerformanceCase{
		{question: "什么是九型人格", types: []int{1, 2, 3, 4, 5, 6, 7, 8, 9}},
		{question: "什么是九型人格", types: []int{1, 2, 3, 4, 5, 6, 7, 8, 9}, stream: true},
		{question: "1 2 3 4 这些型号的反馈", types: []int{1, 2, 3, 4}},
		{question: "1 2 3 4 这些型号的反馈", types: []int{1, 2, 3, 4}, stream: true},
	}
	for typeNumber := 1; typeNumber <= 9; typeNumber++ {
		question := fmt.Sprintf("%d号是什么样的", typeNumber)
		cases = append(cases,
			mainChatPerformanceCase{question: question, types: []int{typeNumber}},
			mainChatPerformanceCase{question: question, types: []int{typeNumber}, stream: true},
		)
	}
	return cases
}

func verifyMainChatPerformanceFixture(ctx context.Context, database *sql.DB, sessionID int64) error {
	var account, registerSource, scene string
	err := database.QueryRowContext(ctx, `
		SELECT COALESCE(u.account,''), COALESCE(u.register_source,''), COALESCE(s.scene,'')
		FROM app_chat_sessions s
		JOIN app_users u ON u.id=s.app_user_id
		WHERE s.id=$1`, sessionID).Scan(&account, &registerSource, &scene)
	if err != nil {
		return fmt.Errorf("look up performance fixture: %w", err)
	}
	isFixture := strings.HasPrefix(strings.ToLower(strings.TrimSpace(account)), "performance_fixture") || strings.EqualFold(strings.TrimSpace(registerSource), "performance_fixture")
	if !isFixture || scene != "chat" {
		return fmt.Errorf("session %d is not owned by a dedicated performance fixture chat account", sessionID)
	}
	return nil
}

func resetMainChatPerformanceFixture(ctx context.Context, database *sql.DB, sessionID int64) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var account, registerSource, scene string
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(u.account,''), COALESCE(u.register_source,''), COALESCE(s.scene,'')
		FROM app_chat_sessions s
		JOIN app_users u ON u.id=s.app_user_id
		WHERE s.id=$1 FOR UPDATE OF s`, sessionID).Scan(&account, &registerSource, &scene); err != nil {
		return err
	}
	isFixture := strings.HasPrefix(strings.ToLower(strings.TrimSpace(account)), "performance_fixture") || strings.EqualFold(strings.TrimSpace(registerSource), "performance_fixture")
	if !isFixture || scene != "chat" {
		return errors.New("performance fixture ownership changed")
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM app_chat_knowledge_traces WHERE session_id=$1`, sessionID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM app_chat_messages WHERE session_id=$1`, sessionID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE app_chat_sessions SET context_summary='', context_summary_through_message_id=0 WHERE id=$1`, sessionID); err != nil {
		return err
	}
	var messages, traces int
	var summary string
	var cursor int64
	if err := tx.QueryRowContext(ctx, `
		SELECT
		  (SELECT count(*) FROM app_chat_messages WHERE session_id=$1),
		  (SELECT count(*) FROM app_chat_knowledge_traces WHERE session_id=$1),
		  context_summary, context_summary_through_message_id
		FROM app_chat_sessions WHERE id=$1`, sessionID).Scan(&messages, &traces, &summary, &cursor); err != nil {
		return err
	}
	if messages != 0 || traces != 0 || summary != "" || cursor != 0 {
		return fmt.Errorf("fixture reset verification failed: messages=%d traces=%d summary_runes=%d cursor=%d", messages, traces, utf8.RuneCountInString(summary), cursor)
	}
	return tx.Commit()
}

func measureMainChatPerformanceRequest(client *http.Client, baseURL, token string, sessionID int64, testCase mainChatPerformanceCase) (mainChatPerformanceMetric, string, error) {
	payload, err := json.Marshal(map[string]string{"question": testCase.question})
	if err != nil {
		return mainChatPerformanceMetric{}, "", err
	}
	path := fmt.Sprintf("%s/api/app/chat/sessions/%d/ask", baseURL, sessionID)
	if testCase.stream {
		path += "/stream"
	}
	request, err := http.NewRequest(http.MethodPost, path, strings.NewReader(string(payload)))
	if err != nil {
		return mainChatPerformanceMetric{}, "", err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	startedAt := time.Now()
	response, err := client.Do(request)
	if err != nil {
		return mainChatPerformanceMetric{}, "", err
	}
	defer response.Body.Close()
	metric := mainChatPerformanceMetric{Timestamp: time.Now().UTC(), Status: response.StatusCode, RequestedTypeCount: len(testCase.types)}
	var answer string
	if testCase.stream {
		answer, metric.TTFTMillis, err = readMainChatPerformanceStream(response.Body, startedAt)
	} else {
		answer, err = readMainChatPerformanceSync(response.Body)
	}
	metric.CompletionMillis = time.Since(startedAt).Milliseconds()
	metric.ResponseRunes = utf8.RuneCountInString(answer)
	metric.CoveredDimensionCount = countMainChatPerformanceDimensions(answer, testCase.types)
	metric.Truncated = strings.TrimSpace(answer) == "" || (len(testCase.types) > 0 && !hasMainChatTerminalPunctuation(answer))
	return metric, answer, err
}

func readMainChatPerformanceSync(reader io.Reader) (string, error) {
	var response struct {
		Data struct {
			Answer string `json:"answer"`
		} `json:"data"`
	}
	err := json.NewDecoder(io.LimitReader(reader, 4<<20)).Decode(&response)
	return response.Data.Answer, err
}

func readMainChatPerformanceStream(reader io.Reader, startedAt time.Time) (string, int64, error) {
	scanner := bufio.NewScanner(io.LimitReader(reader, 4<<20))
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	var event, data string
	var answer strings.Builder
	var finalAnswer string
	var ttft int64
	process := func() error {
		switch event {
		case "delta":
			var payload struct {
				Content string `json:"content"`
			}
			if err := json.Unmarshal([]byte(data), &payload); err != nil {
				return err
			}
			if payload.Content != "" && ttft == 0 {
				ttft = time.Since(startedAt).Milliseconds()
				if ttft == 0 {
					ttft = 1
				}
			}
			answer.WriteString(payload.Content)
		case "done":
			var payload struct {
				Answer string `json:"answer"`
			}
			if err := json.Unmarshal([]byte(data), &payload); err != nil {
				return err
			}
			finalAnswer = payload.Answer
		case "error":
			return errors.New("stream returned error event")
		}
		return nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := process(); err != nil {
				return answer.String(), ttft, err
			}
			event, data = "", ""
			continue
		}
		if strings.HasPrefix(line, "event: ") {
			event = strings.TrimSpace(strings.TrimPrefix(line, "event: "))
		} else if strings.HasPrefix(line, "data: ") {
			data = strings.TrimSpace(strings.TrimPrefix(line, "data: "))
		}
	}
	if err := scanner.Err(); err != nil {
		return answer.String(), ttft, err
	}
	if finalAnswer != "" {
		return finalAnswer, ttft, nil
	}
	return answer.String(), ttft, nil
}

func validateMainChatPerformanceAnswer(t *testing.T, testCase mainChatPerformanceCase, metric mainChatPerformanceMetric, answer string) {
	t.Helper()
	if metric.Status < 200 || metric.Status >= 300 {
		t.Errorf("HTTP status = %d for stream=%v requested_types=%d", metric.Status, testCase.stream, len(testCase.types))
	}
	if len(testCase.types) == 0 {
		return
	}
	for _, typeNumber := range testCase.types {
		heading := appChatEnneagramCanonicalNames[typeNumber-1]
		if !strings.Contains(answer, heading) {
			t.Errorf("answer missing canonical heading %q", heading)
		}
	}
	if metric.CoveredDimensionCount != len(testCase.types)*len(appChatEnneagramDimensionContracts) {
		t.Errorf("covered dimensions = %d, want %d", metric.CoveredDimensionCount, len(testCase.types)*len(appChatEnneagramDimensionContracts))
	}
	if metric.Truncated {
		t.Error("answer appears truncated")
	}
}

func countMainChatPerformanceDimensions(answer string, requestedTypes []int) int {
	count := 0
	for index, typeNumber := range requestedTypes {
		heading := appChatEnneagramCanonicalNames[typeNumber-1]
		start := strings.Index(answer, heading)
		if start < 0 {
			continue
		}
		end := len(answer)
		if index+1 < len(requestedTypes) {
			nextHeading := appChatEnneagramCanonicalNames[requestedTypes[index+1]-1]
			if relative := strings.Index(answer[start+len(heading):], nextHeading); relative >= 0 {
				end = start + len(heading) + relative
			}
		}
		section := answer[start:end]
		for _, dimension := range appChatEnneagramDimensionContracts {
			if strings.Contains(section, dimension.Name) {
				count++
			}
		}
	}
	return count
}

func hasMainChatTerminalPunctuation(answer string) bool {
	answer = strings.TrimSpace(answer)
	return strings.HasSuffix(answer, "。") || strings.HasSuffix(answer, "！") || strings.HasSuffix(answer, "？") ||
		strings.HasSuffix(answer, ".") || strings.HasSuffix(answer, "!") || strings.HasSuffix(answer, "?")
}

func readMainChatPerformanceMetrics(t *testing.T, path string) []mainChatPerformanceMetric {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var metrics []mainChatPerformanceMetric
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var metric mainChatPerformanceMetric
		if err := json.Unmarshal(scanner.Bytes(), &metric); err != nil {
			t.Fatalf("decode baseline metric: %v", err)
		}
		metrics = append(metrics, metric)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return metrics
}

func applyMainChatPerformanceGates(t *testing.T, candidate, baseline []mainChatPerformanceMetric) {
	t.Helper()
	var candidateTTFT, candidateOverviewTTFT, baselineTTFT, overviewCompletion []int64
	for _, metric := range candidate {
		if metric.Status < 200 || metric.Status >= 300 || metric.Truncated {
			t.Errorf("candidate functional metric failed: status=%d truncated=%v requested_types=%d", metric.Status, metric.Truncated, metric.RequestedTypeCount)
		}
		if metric.TTFTMillis > 0 {
			candidateTTFT = append(candidateTTFT, metric.TTFTMillis)
			if metric.RequestedTypeCount == 9 {
				candidateOverviewTTFT = append(candidateOverviewTTFT, metric.TTFTMillis)
			}
		}
		if metric.RequestedTypeCount == 9 {
			overviewCompletion = append(overviewCompletion, metric.CompletionMillis)
		}
	}
	for _, metric := range baseline {
		if metric.TTFTMillis > 0 && metric.RequestedTypeCount == 9 {
			baselineTTFT = append(baselineTTFT, metric.TTFTMillis)
		}
	}
	candidateP95 := mainChatPerformanceP95(candidateTTFT)
	candidateOverviewP95 := mainChatPerformanceP95(candidateOverviewTTFT)
	baselineP95 := mainChatPerformanceP95(baselineTTFT)
	if candidateP95 == 0 || candidateOverviewP95 == 0 || baselineP95 == 0 {
		t.Error("candidate or baseline streaming TTFT samples are missing")
	} else {
		if candidateP95 >= 2500 {
			t.Errorf("streaming TTFT p95 = %dms, want <2500ms", candidateP95)
		}
		if candidateOverviewP95 > baselineP95+300 {
			t.Errorf("overview streaming TTFT p95 regression = %dms, baseline=%dms limit=%dms", candidateOverviewP95, baselineP95, baselineP95+300)
		}
	}
	if completionP95 := mainChatPerformanceP95(overviewCompletion); completionP95 == 0 || completionP95 >= 45000 {
		t.Errorf("all-nine completion p95 = %dms, want <45000ms", completionP95)
	}
}

func mainChatPerformanceP95(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	values = append([]int64(nil), values...)
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	index := (95*len(values) + 99) / 100
	if index < 1 {
		index = 1
	}
	return values[index-1]
}
