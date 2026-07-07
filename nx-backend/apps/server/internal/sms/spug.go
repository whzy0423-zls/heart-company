package sms

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type SpugOptions struct {
	APIBase        string
	TemplateCode   string
	TemplateName   string
	CodeTTLMinutes int
	Timeout        time.Duration
	HTTPClient     *http.Client
}

type SpugSender struct {
	apiBase        string
	templateCode   string
	templateName   string
	codeTTLMinutes int
	timeout        time.Duration
	httpClient     *http.Client
}

func NewSpugSender(opts SpugOptions) (*SpugSender, error) {
	apiBase := strings.TrimRight(strings.TrimSpace(opts.APIBase), "/")
	if apiBase == "" {
		apiBase = "https://push.spug.cc"
	}
	parsed, err := url.Parse(apiBase)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("sms: invalid spug api base")
	}

	templateCode := strings.TrimSpace(opts.TemplateCode)
	if templateCode == "" {
		return nil, fmt.Errorf("sms: spug template code is required")
	}
	templateName := strings.TrimSpace(opts.TemplateName)
	if templateName == "" {
		templateName = "芯之力"
	}

	codeTTLMinutes := opts.CodeTTLMinutes
	if codeTTLMinutes <= 0 {
		codeTTLMinutes = 10
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}

	return &SpugSender{
		apiBase:        apiBase,
		templateCode:   templateCode,
		templateName:   templateName,
		codeTTLMinutes: codeTTLMinutes,
		timeout:        timeout,
		httpClient:     httpClient,
	}, nil
}

func (s *SpugSender) Send(ctx context.Context, phone, code string) error {
	if s.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.timeout)
		defer cancel()
	}

	endpoint, err := url.Parse(s.apiBase + "/sms/" + url.PathEscape(s.templateCode))
	if err != nil {
		return fmt.Errorf("sms: create spug url: %w", err)
	}
	query := endpoint.Query()
	query.Set("to", phone)
	query.Set("code", code)
	query.Set("number", strconv.Itoa(s.codeTTLMinutes))
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("sms: create spug request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sms: spug send failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := readProviderBody(resp.Body)
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("sms: spug provider rejected: status=%d body=%s", resp.StatusCode, msg)
	}
	return nil
}

func readProviderBody(body io.Reader) string {
	b, err := io.ReadAll(io.LimitReader(body, 512))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
