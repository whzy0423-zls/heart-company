package location

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/netguard"
)

const (
	amapHost        = "restapi.amap.com"
	amapBaseURL     = "https://" + amapHost
	amapPlacePath   = "/v3/place/text"
	amapReversePath = "/v3/geocode/regeo"
)

// Exported endpoint names are useful to integration tests and diagnostics;
// callers cannot override them through Config.
const (
	AMapHost        = amapHost
	AMapPlacePath   = amapPlacePath
	AMapReversePath = amapReversePath
)

// HTTPDoer is the small seam used to test provider behavior without making a
// network call.  *http.Client satisfies it.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// Config configures the AMap Web Service client.  APIKey is intentionally the
// Web Service key and is never included in an error returned by this package.
// Doer takes precedence over HTTPClient and is primarily useful for tests.
type Config struct {
	APIKey           string
	HTTPClient       *http.Client
	Doer             HTTPDoer
	Timeout          time.Duration
	MaxResponseBytes int64
}

// AMapConfig is an alias retained for callers that prefer provider-qualified
// configuration names.
type AMapConfig = Config

// Client proxies the fixed AMap place and reverse-geocoding endpoints.
type Client struct {
	apiKey           string
	doer             HTTPDoer
	timeout          time.Duration
	maxResponseBytes int64
}

// AMapClient is an alias for Client.
type AMapClient = Client

// NewAMapClient creates a client.  Missing configuration is reported by the
// first operation as ErrNotConfigured so callers can construct dependencies in
// every environment and map the failure consistently.
func NewAMapClient(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultRequestTimeout
	} else if timeout > DefaultRequestTimeout {
		// The upstream contract has a hard four-second ceiling.  A caller may
		// choose a shorter timeout for a test or a tighter request budget, but
		// cannot widen the provider deadline.
		timeout = DefaultRequestTimeout
	}
	maxResponseBytes := cfg.MaxResponseBytes
	if maxResponseBytes <= 0 {
		maxResponseBytes = MaxUpstreamResponseBytes
	} else if maxResponseBytes > MaxUpstreamResponseBytes {
		// Keep the response memory boundary fixed even when a custom client is
		// assembled by an integration.
		maxResponseBytes = MaxUpstreamResponseBytes
	}

	var doer HTTPDoer
	switch {
	case cfg.Doer != nil:
		doer = cfg.Doer
	case cfg.HTTPClient != nil:
		// Copy the supplied client so its redirect policy cannot weaken the
		// provider boundary and so callers retain ownership of their client.
		copyOfClient := *cfg.HTTPClient
		copyOfClient.Timeout = timeout
		copyOfClient.CheckRedirect = rejectRedirects
		doer = &copyOfClient
	default:
		doer = &http.Client{
			Timeout:       timeout,
			Transport:     netguard.NewGuardedTransport(),
			CheckRedirect: rejectRedirects,
		}
	}

	return &Client{
		apiKey:           strings.TrimSpace(cfg.APIKey),
		doer:             doer,
		timeout:          timeout,
		maxResponseBytes: maxResponseBytes,
	}
}

// NewClient is a short constructor alias matching other server provider
// packages.
func NewClient(cfg Config) *Client {
	return NewAMapClient(cfg)
}

// Configured reports whether a non-empty Web Service key is available.
func (c *Client) Configured() bool {
	return c != nil && c.apiKey != ""
}

// Enabled is synonymous with Configured and lets callers use the same probe
// convention as other provider clients.
func (c *Client) Enabled() bool {
	return c.Configured()
}

// Search performs a text place search and returns bounded candidates.
func (c *Client) Search(ctx context.Context, input SearchInput) ([]Candidate, error) {
	if !c.Configured() {
		return nil, ErrNotConfigured
	}
	normalized, err := NormalizeSearchInput(input)
	if err != nil {
		return nil, ErrInvalidRequest
	}
	params := url.Values{
		"key":        {c.apiKey},
		"keywords":   {normalized.Query},
		"extensions": {"all"},
		"output":     {"json"},
	}
	if normalized.City != "" {
		params.Set("city", normalized.City)
	}
	raw, err := c.get(ctx, amapPlacePath, params)
	if err != nil {
		return nil, err
	}

	var payload amapSearchResponse
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, ErrUnavailable
	}
	status, ok := rawString(payload.Status)
	if !ok || status != "1" {
		return nil, ErrUnavailable
	}
	items, ok := rawObjectSlice(payload.Pois)
	if !ok {
		return nil, ErrUnavailable
	}
	result := make([]Candidate, 0, minInt(len(items), MaxCandidates))
	for _, item := range items {
		candidate, ok := candidateFromPOI(item)
		if !ok {
			continue
		}
		result = append(result, candidate)
		if len(result) >= MaxCandidates {
			break
		}
	}
	return NormalizeCandidates(result), nil
}

// Reverse performs a reverse-geocoding lookup.  A valid provider response
// without a usable address is represented by (nil, nil), not an error.
func (c *Client) Reverse(ctx context.Context, input ReverseInput) (*Candidate, error) {
	if !c.Configured() {
		return nil, ErrNotConfigured
	}
	normalized, err := NormalizeReverseInput(input)
	if err != nil {
		return nil, ErrInvalidRequest
	}
	params := url.Values{
		"key":        {c.apiKey},
		"location":   {formatCoordinate(normalized.Longitude) + "," + formatCoordinate(normalized.Latitude)},
		"extensions": {"all"},
		"radius":     {"1000"},
		"roadlevel":  {"0"},
		"output":     {"json"},
	}
	raw, err := c.get(ctx, amapReversePath, params)
	if err != nil {
		return nil, err
	}

	var payload amapReverseResponse
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, ErrUnavailable
	}
	status, ok := rawString(payload.Status)
	if !ok || status != "1" {
		return nil, ErrUnavailable
	}
	if isNullOrEmpty(payload.Regeocode) {
		return nil, nil
	}
	regeocode, ok := rawObject(payload.Regeocode)
	if !ok {
		return nil, ErrUnavailable
	}
	candidate := reverseCandidate(regeocode, normalized)
	if candidate == nil {
		return nil, nil
	}
	return candidate, nil
}

func (c *Client) get(ctx context.Context, path string, params url.Values) ([]byte, error) {
	if c == nil || c.doer == nil {
		return nil, ErrUnavailable
	}
	timeout := c.timeout
	if timeout <= 0 {
		timeout = DefaultRequestTimeout
	} else if timeout > DefaultRequestTimeout {
		timeout = DefaultRequestTimeout
	}
	maxResponseBytes := c.maxResponseBytes
	if maxResponseBytes <= 0 {
		maxResponseBytes = MaxUpstreamResponseBytes
	} else if maxResponseBytes > MaxUpstreamResponseBytes {
		maxResponseBytes = MaxUpstreamResponseBytes
	}
	if ctx == nil {
		ctx = context.Background()
	}
	requestContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	endpoint := amapBaseURL + path
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, ErrUnavailable
	}
	parsed.RawQuery = params.Encode()
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, ErrUnavailable
	}
	request.Header.Set("Accept", "application/json")
	response, err := c.doer.Do(request)
	if err != nil {
		if isTimeoutError(requestContext, err) {
			return nil, ErrTimeout
		}
		return nil, ErrUnavailable
	}
	if response == nil {
		return nil, ErrUnavailable
	}
	if response.Body != nil {
		defer response.Body.Close()
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices || response.Body == nil {
		return nil, ErrUnavailable
	}
	if response.ContentLength < -1 || response.ContentLength > maxResponseBytes {
		return nil, ErrUnavailable
	}
	limited := io.LimitReader(response.Body, maxResponseBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		if isTimeoutError(requestContext, err) {
			return nil, ErrTimeout
		}
		return nil, ErrUnavailable
	}
	if int64(len(body)) > maxResponseBytes {
		return nil, ErrUnavailable
	}
	return body, nil
}

func rejectRedirects(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}

func isTimeoutError(ctx context.Context, err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

type amapSearchResponse struct {
	Status json.RawMessage `json:"status"`
	Pois   json.RawMessage `json:"pois"`
}

type amapReverseResponse struct {
	Status    json.RawMessage `json:"status"`
	Regeocode json.RawMessage `json:"regeocode"`
}

func candidateFromPOI(item map[string]json.RawMessage) (Candidate, bool) {
	name, ok := fieldString(item, "name")
	if !ok {
		return Candidate{}, false
	}
	address, ok := fieldString(item, "address")
	if !ok {
		return Candidate{}, false
	}
	province, ok := fieldString(item, "pname")
	if !ok {
		return Candidate{}, false
	}
	city, ok := fieldString(item, "cityname")
	if !ok {
		return Candidate{}, false
	}
	district, ok := fieldString(item, "adname")
	if !ok {
		return Candidate{}, false
	}
	latitude, longitude, ok := poiCoordinate(item)
	if !ok {
		return Candidate{}, false
	}
	normalized, err := NormalizeCandidate(Candidate{
		Name:             name,
		Address:          address,
		Province:         province,
		City:             city,
		District:         district,
		Latitude:         latitude,
		Longitude:        longitude,
		CoordinateSystem: CoordinateSystemGCJ02,
	})
	if err != nil {
		return Candidate{}, false
	}
	return normalized, true
}

func poiCoordinate(item map[string]json.RawMessage) (float64, float64, bool) {
	if location, ok := fieldString(item, "location"); ok && strings.TrimSpace(location) != "" {
		parts := strings.Split(location, ",")
		if len(parts) == 2 {
			longitude, longitudeErr := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
			latitude, latitudeErr := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			if longitudeErr == nil && latitudeErr == nil && validLatitude(latitude) && validLongitude(longitude) {
				return latitude, longitude, true
			}
		}
		return 0, 0, false
	}
	latitude, latitudeOK := fieldFloat(item, "latitude")
	longitude, longitudeOK := fieldFloat(item, "longitude")
	if latitudeOK && longitudeOK && validLatitude(latitude) && validLongitude(longitude) {
		return latitude, longitude, true
	}
	return 0, 0, false
}

func reverseCandidate(regeocode map[string]json.RawMessage, input ReverseInput) *Candidate {
	formatted, _ := fieldString(regeocode, "formatted_address")
	province, city, district, township, street := reverseAddressFields(regeocode)

	var poiName, poiAddress, roadName string
	if pois, ok := rawObjectSlice(regeocode["pois"]); ok {
		for _, poi := range pois {
			name, nameOK := fieldString(poi, "name")
			address, addressOK := fieldString(poi, "address")
			if !nameOK || !addressOK {
				continue
			}
			if poiName == "" {
				poiName = name
			}
			if poiAddress == "" {
				poiAddress = address
			}
			if poiName != "" && poiAddress != "" {
				break
			}
		}
	}
	if roads, ok := rawObjectSlice(regeocode["roads"]); ok {
		for _, road := range roads {
			name, nameOK := fieldString(road, "name")
			if nameOK && name != "" {
				roadName = name
				break
			}
		}
	}

	name := firstNonEmpty(poiName, roadName, street, township, district)
	address := firstNonEmpty(formatted, poiAddress, joinAddress(province, city, district, township, street))
	if name == "" && address == "" && province == "" && city == "" && district == "" {
		return nil
	}
	normalized, err := NormalizeCandidate(Candidate{
		Name:             name,
		Address:          address,
		Province:         province,
		City:             city,
		District:         district,
		Latitude:         input.Latitude,
		Longitude:        input.Longitude,
		CoordinateSystem: CoordinateSystemGCJ02,
	})
	if err != nil {
		return nil
	}
	return &normalized
}

func reverseAddressFields(regeocode map[string]json.RawMessage) (string, string, string, string, string) {
	component, ok := rawObject(regeocode["addressComponent"])
	if !ok {
		return "", "", "", "", ""
	}
	province, _ := fieldString(component, "province")
	city, _ := fieldString(component, "city")
	district, _ := fieldString(component, "district")
	township, _ := fieldString(component, "township")
	street := ""
	if streetNumber, ok := rawObject(component["streetNumber"]); ok {
		street, _ = fieldString(streetNumber, "street")
		if street == "" {
			street, _ = fieldString(streetNumber, "number")
		}
	}
	return province, city, district, township, street
}

func fieldString(object map[string]json.RawMessage, key string) (string, bool) {
	value, exists := object[key]
	if !exists {
		return "", true
	}
	return rawString(value)
}

func rawString(raw json.RawMessage) (string, bool) {
	if isNullOrEmpty(raw) {
		return "", true
	}
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		return value, true
	}
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err == nil {
		for _, item := range values {
			value, ok := rawString(item)
			if ok && value != "" {
				return value, true
			}
		}
		return "", true
	}
	return "", false
}

func fieldFloat(object map[string]json.RawMessage, key string) (float64, bool) {
	value, exists := object[key]
	if !exists || isNullOrEmpty(value) {
		return 0, false
	}
	var number float64
	if err := json.Unmarshal(value, &number); err == nil {
		return number, true
	}
	var text string
	if err := json.Unmarshal(value, &text); err != nil {
		return 0, false
	}
	number, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	return number, err == nil
}

func rawObject(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	if isNullOrEmpty(raw) {
		return nil, true
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return nil, false
	}
	return object, true
}

func rawObjectSlice(raw json.RawMessage) ([]map[string]json.RawMessage, bool) {
	if isNullOrEmpty(raw) {
		return nil, true
	}
	var objects []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &objects); err != nil {
		return nil, false
	}
	return objects, true
}

func isNullOrEmpty(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return true
	}
	var emptyString string
	return json.Unmarshal([]byte(trimmed), &emptyString) == nil && strings.TrimSpace(emptyString) == ""
}

func formatCoordinate(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func joinAddress(values ...string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if len(parts) == 0 || parts[len(parts)-1] != value {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, "")
}
