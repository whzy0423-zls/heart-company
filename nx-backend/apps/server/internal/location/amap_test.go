package location

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

type recordingDoer struct {
	mu       sync.Mutex
	requests []*http.Request
	response *http.Response
	err      error
}

func (d *recordingDoer) Do(req *http.Request) (*http.Response, error) {
	d.mu.Lock()
	d.requests = append(d.requests, req.Clone(req.Context()))
	d.mu.Unlock()
	if d.err != nil {
		return nil, d.err
	}
	return d.response, nil
}

func (d *recordingDoer) lastRequest() *http.Request {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.requests) == 0 {
		return nil
	}
	return d.requests[len(d.requests)-1]
}

type contextDoer struct{}

func (contextDoer) Do(req *http.Request) (*http.Response, error) {
	<-req.Context().Done()
	return nil, req.Context().Err()
}

type closeTrackingReadCloser struct {
	io.Reader
	closed bool
}

func (r *closeTrackingReadCloser) Close() error {
	r.closed = true
	return nil
}

func TestSearchUsesFixedHTTPSHostAndPlacePath(t *testing.T) {
	doer := &recordingDoer{response: jsonResponse(`{"status":"1","count":"1","pois":[{"name":"深圳大学城","address":"留仙大道","pname":"广东省","cityname":"深圳市","adname":"南山区","location":"113.9734,22.5901"}]}`)}
	client := NewAMapClient(Config{APIKey: "SECRET_KEY", Doer: doer})

	items, err := client.Search(context.Background(), SearchInput{Query: "深圳大学城", City: "深圳市"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(items) != 1 || items[0].Name != "深圳大学城" || items[0].Latitude != 22.5901 || items[0].Longitude != 113.9734 {
		t.Fatalf("unexpected candidates: %+v", items)
	}
	req := doer.lastRequest()
	if req == nil {
		t.Fatal("expected an upstream request")
	}
	if req.Method != http.MethodGet || req.URL.Scheme != "https" || req.URL.Host != amapHost || req.URL.Path != amapPlacePath {
		t.Fatalf("request = %s %s, want GET https://%s%s", req.Method, req.URL, amapHost, amapPlacePath)
	}
	values := req.URL.Query()
	if values.Get("key") != "SECRET_KEY" || values.Get("keywords") != "深圳大学城" || values.Get("city") != "深圳市" {
		t.Fatalf("query parameters = %v", values)
	}
	if values.Get("output") != "json" {
		t.Fatalf("output = %q, want json", values.Get("output"))
	}
}

func TestReverseUsesFixedHTTPSGeocodePathAndGCJ02Order(t *testing.T) {
	doer := &recordingDoer{response: jsonResponse(`{"status":"1","regeocode":{"formatted_address":"广东省深圳市南山区留仙大道","addressComponent":{"province":"广东省","city":"深圳市","district":"南山区"},"pois":[{"name":"深圳大学城","address":"留仙大道"}]}}`)}
	client := NewAMapClient(Config{APIKey: "SECRET_KEY", Doer: doer})

	got, err := client.Reverse(context.Background(), ReverseInput{Latitude: 22.5901, Longitude: 113.9734, CoordinateSystem: CoordinateSystemGCJ02})
	if err != nil {
		t.Fatalf("Reverse() error = %v", err)
	}
	if got == nil || got.Name != "深圳大学城" || got.Address != "广东省深圳市南山区留仙大道" || got.Latitude != 22.5901 || got.Longitude != 113.9734 {
		t.Fatalf("unexpected reverse candidate: %+v", got)
	}
	req := doer.lastRequest()
	if req == nil || req.Method != http.MethodGet || req.URL.Scheme != "https" || req.URL.Host != amapHost || req.URL.Path != amapReversePath {
		t.Fatalf("request = %v, want fixed reverse endpoint", req)
	}
	if gotLocation := req.URL.Query().Get("location"); gotLocation != "113.9734,22.5901" {
		t.Fatalf("location = %q, want longitude,latitude", gotLocation)
	}
}

func TestEmptyUpstreamResultsRemainSuccessfulEmptyValues(t *testing.T) {
	searchDoer := &recordingDoer{response: jsonResponse(`{"status":"1","count":"0","pois":[]}`)}
	client := NewAMapClient(Config{APIKey: "SECRET_KEY", Doer: searchDoer})
	items, err := client.Search(context.Background(), SearchInput{Query: "不存在"})
	if err != nil || len(items) != 0 {
		t.Fatalf("empty search = (%v, %v), want ([], nil)", items, err)
	}

	reverseDoer := &recordingDoer{response: jsonResponse(`{"status":"1","regeocode":{"formatted_address":"","addressComponent":{},"pois":[]}}`)}
	client = NewAMapClient(Config{APIKey: "SECRET_KEY", Doer: reverseDoer})
	candidate, err := client.Reverse(context.Background(), ReverseInput{Latitude: 0, Longitude: 0, CoordinateSystem: CoordinateSystemGCJ02})
	if err != nil || candidate != nil {
		t.Fatalf("empty reverse = (%v, %v), want (nil, nil)", candidate, err)
	}

	client = NewAMapClient(Config{APIKey: "SECRET_KEY", Doer: &recordingDoer{response: jsonResponse(`{"status":"1","regeocode":""}`)}})
	candidate, err = client.Reverse(context.Background(), ReverseInput{Latitude: 0, Longitude: 0, CoordinateSystem: CoordinateSystemGCJ02})
	if err != nil || candidate != nil {
		t.Fatalf("empty-string reverse = (%v, %v), want (nil, nil)", candidate, err)
	}
}

func TestSearchCapsCandidatesAndFieldsFromUpstream(t *testing.T) {
	var pois strings.Builder
	pois.WriteString(`{"status":"1","pois":[`)
	for i := 0; i < MaxCandidates+5; i++ {
		if i > 0 {
			pois.WriteByte(',')
		}
		fmt.Fprintf(&pois, `{"name":%q,"address":%q,"pname":%q,"cityname":%q,"adname":%q,"location":"113.9,22.5"}`,
			strings.Repeat("名", MaxCandidateFieldCodePoints+10),
			strings.Repeat("址", MaxCandidateFieldCodePoints+10),
			strings.Repeat("省", MaxCandidateFieldCodePoints+10),
			strings.Repeat("市", MaxCandidateFieldCodePoints+10),
			strings.Repeat("区", MaxCandidateFieldCodePoints+10),
		)
	}
	pois.WriteString(`]}`)
	client := NewAMapClient(Config{APIKey: "SECRET_KEY", Doer: &recordingDoer{response: jsonResponse(pois.String())}})
	items, err := client.Search(context.Background(), SearchInput{Query: "学校"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(items) != MaxCandidates {
		t.Fatalf("candidate count = %d, want %d", len(items), MaxCandidates)
	}
	if len([]rune(items[0].Name)) != MaxCandidateFieldCodePoints || len([]rune(items[0].Address)) != MaxCandidateFieldCodePoints {
		t.Fatalf("candidate field limits not enforced: name=%d address=%d", len([]rune(items[0].Name)), len([]rune(items[0].Address)))
	}
}

func TestClientReturnsOnlyStableErrorsWithoutUpstreamSecrets(t *testing.T) {
	tests := []struct {
		name  string
		setup func() *Client
		call  func(*Client) error
		want  error
	}{
		{
			name: "not configured",
			setup: func() *Client {
				return NewAMapClient(Config{Doer: &recordingDoer{}})
			},
			call: func(c *Client) error {
				_, err := c.Search(context.Background(), SearchInput{Query: "秘密关键词"})
				return err
			},
			want: ErrNotConfigured,
		},
		{
			name: "non success status",
			setup: func() *Client {
				return NewAMapClient(Config{APIKey: "SECRET_KEY", Doer: &recordingDoer{response: &http.Response{StatusCode: http.StatusForbidden, Body: io.NopCloser(strings.NewReader("SECRET_KEY https://restapi.amap.com/v3/place/text?keywords=秘密关键词"))}}})
			},
			call: func(c *Client) error {
				_, err := c.Search(context.Background(), SearchInput{Query: "秘密关键词"})
				return err
			},
			want: ErrUnavailable,
		},
		{
			name: "malformed json",
			setup: func() *Client {
				return NewAMapClient(Config{APIKey: "SECRET_KEY", Doer: &recordingDoer{response: jsonResponse("{not-json")}})
			},
			call: func(c *Client) error {
				_, err := c.Search(context.Background(), SearchInput{Query: "秘密关键词"})
				return err
			},
			want: ErrUnavailable,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call(tt.setup())
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want errors.Is(..., %v)", err, tt.want)
			}
			for _, secret := range []string{"SECRET_KEY", "秘密关键词", "restapi.amap.com", "v3/place/text"} {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("error leaked %q: %v", secret, err)
				}
			}
		})
	}
}

func TestClientMapsContextDeadlineAndTransportTimeoutToErrTimeout(t *testing.T) {
	client := NewAMapClient(Config{APIKey: "SECRET_KEY", Doer: contextDoer{}, Timeout: 20 * time.Millisecond})
	_, err := client.Search(context.Background(), SearchInput{Query: "学校"})
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("error = %v, want ErrTimeout", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	_, err = client.Search(ctx, SearchInput{Query: "学校"})
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("deadline error = %v, want ErrTimeout", err)
	}
}

func TestClientRejectsResponsesOver256KiBAndClosesBody(t *testing.T) {
	body := &closeTrackingReadCloser{Reader: strings.NewReader(strings.Repeat("x", MaxUpstreamResponseBytes+1))}
	doer := &recordingDoer{response: &http.Response{StatusCode: http.StatusOK, Body: body}}
	client := NewAMapClient(Config{APIKey: "SECRET_KEY", Doer: doer})
	_, err := client.Search(context.Background(), SearchInput{Query: "学校"})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
	if !body.closed {
		t.Fatal("oversize response body was not closed")
	}

	invalidLengthBody := &closeTrackingReadCloser{Reader: strings.NewReader(`{"status":"1","pois":[]}`)}
	client = NewAMapClient(Config{APIKey: "SECRET_KEY", Doer: &recordingDoer{response: &http.Response{
		StatusCode:    http.StatusOK,
		ContentLength: -2,
		Body:          invalidLengthBody,
	}}})
	_, err = client.Search(context.Background(), SearchInput{Query: "学校"})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("invalid negative ContentLength error = %v, want ErrUnavailable", err)
	}
	if !invalidLengthBody.closed {
		t.Fatal("invalid ContentLength response body was not closed")
	}
}

func TestClientDoesNotFollowRedirects(t *testing.T) {
	client := NewAMapClient(Config{APIKey: "SECRET_KEY"})
	httpClient, ok := client.doer.(*http.Client)
	if !ok {
		t.Fatalf("default doer type = %T, want *http.Client", client.doer)
	}
	if httpClient.CheckRedirect == nil {
		t.Fatal("default HTTP client must disable redirects")
	}
	redirectReq, _ := http.NewRequest(http.MethodGet, "https://restapi.amap.com/redirect", nil)
	if err := httpClient.CheckRedirect(redirectReq, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("CheckRedirect() = %v, want http.ErrUseLastResponse", err)
	}
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func _assertURLValues(t *testing.T, raw string, expected url.Values) {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	for key, values := range expected {
		if parsed.Query().Get(key) != values[0] {
			t.Fatalf("query %s = %q, want %q", key, parsed.Query().Get(key), values[0])
		}
	}
}
