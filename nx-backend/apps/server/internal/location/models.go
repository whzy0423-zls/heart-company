// Package location contains the short-lived location lookup domain used by
// the app location proxy.  The package deliberately has no persistence
// dependencies: coordinates and provider responses live only for the request
// that needs them.
package location

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"
	"unicode"
)

const (
	// CoordinateSystemGCJ02 is the only coordinate system accepted at the app
	// boundary.  WGS-84 values must be converted by the native location layer
	// before they reach this package.
	CoordinateSystemGCJ02 = "gcj02"

	MaxSearchQueryCodePoints    = 80
	MaxCityCodePoints           = 40
	MaxCandidateFieldCodePoints = 120
	MaxCandidates               = 20
	MaxUpstreamResponseBytes    = 256 * 1024
	DefaultRequestTimeout       = 4 * time.Second
)

// Common aliases make the limits easy to consume from handlers while keeping
// the names used by older callers source-compatible.
const (
	MaxSearchQueryRunes    = MaxSearchQueryCodePoints
	MaxCityRunes           = MaxCityCodePoints
	MaxCandidateRunes      = MaxCandidateFieldCodePoints
	MaxLocationCandidates  = MaxCandidates
	MaxResponseBytes       = MaxUpstreamResponseBytes
	LocationRequestTimeout = DefaultRequestTimeout
)

var (
	// These are the only errors returned by the provider client.  Callers can
	// map them to stable API error codes without exposing provider details.
	ErrNotConfigured  = errors.New("location service not configured")
	ErrInvalidRequest = errors.New("invalid location request")
	ErrTimeout        = errors.New("location provider timeout")
	ErrUnavailable    = errors.New("location provider unavailable")
)

// Service is the provider-independent contract consumed by app handlers.
type Service interface {
	Search(context.Context, SearchInput) ([]Candidate, error)
	Reverse(context.Context, ReverseInput) (*Candidate, error)
}

// SearchInput is the normalized app search request.
type SearchInput struct {
	Query string `json:"query"`
	City  string `json:"city,omitempty"`
}

// ReverseInput is a temporary coordinate lookup request.  Coordinates are
// never persisted by this package.
type ReverseInput struct {
	Latitude         float64 `json:"latitude"`
	Longitude        float64 `json:"longitude"`
	CoordinateSystem string  `json:"coordinateSystem"`
}

// Candidate is the small provider-neutral shape returned to the app.
type Candidate struct {
	Name             string  `json:"name"`
	Address          string  `json:"address"`
	Province         string  `json:"province"`
	City             string  `json:"city"`
	District         string  `json:"district"`
	Latitude         float64 `json:"latitude"`
	Longitude        float64 `json:"longitude"`
	CoordinateSystem string  `json:"coordinateSystem"`
}

// NormalizeSearchInput removes Unicode control characters and validates the
// code-point limits used by the public API.  It returns a copy and never
// modifies the caller's value.
func NormalizeSearchInput(input SearchInput) (SearchInput, error) {
	input.Query = cleanInput(input.Query)
	input.City = cleanInput(input.City)
	if runeCount(input.Query) < 1 || runeCount(input.Query) > MaxSearchQueryCodePoints {
		return SearchInput{}, ErrInvalidRequest
	}
	if runeCount(input.City) > MaxCityCodePoints {
		return SearchInput{}, ErrInvalidRequest
	}
	return input, nil
}

// Validate checks the search input after normalization without exposing any
// provider-specific details.
func (input SearchInput) Validate() error {
	_, err := NormalizeSearchInput(input)
	return err
}

// Normalize is a convenience method equivalent to NormalizeSearchInput.
func (input SearchInput) Normalize() (SearchInput, error) {
	return NormalizeSearchInput(input)
}

// NormalizeReverseInput validates finite GCJ-02 coordinates and canonicalizes
// surrounding whitespace in the coordinate-system marker.
func NormalizeReverseInput(input ReverseInput) (ReverseInput, error) {
	input.CoordinateSystem = strings.TrimSpace(input.CoordinateSystem)
	if input.CoordinateSystem != CoordinateSystemGCJ02 ||
		!validLatitude(input.Latitude) || !validLongitude(input.Longitude) {
		return ReverseInput{}, ErrInvalidRequest
	}
	return input, nil
}

// Validate checks a reverse request.
func (input ReverseInput) Validate() error {
	_, err := NormalizeReverseInput(input)
	return err
}

// NormalizeCandidate strips controls, caps user-visible fields by Unicode
// code point, and validates the temporary coordinate.
func NormalizeCandidate(input Candidate) (Candidate, error) {
	if input.CoordinateSystem != CoordinateSystemGCJ02 ||
		!validLatitude(input.Latitude) || !validLongitude(input.Longitude) {
		return Candidate{}, ErrInvalidRequest
	}
	input.Name = cleanField(input.Name)
	input.Address = cleanField(input.Address)
	input.Province = cleanField(input.Province)
	input.City = cleanField(input.City)
	input.District = cleanField(input.District)
	return input, nil
}

// NormalizeCandidates returns at most MaxCandidates valid, bounded
// candidates.  Malformed provider items are discarded rather than allowed to
// cross the app boundary.
func NormalizeCandidates(items []Candidate) []Candidate {
	result := make([]Candidate, 0, minInt(len(items), MaxCandidates))
	for _, item := range items {
		if len(result) >= MaxCandidates {
			break
		}
		normalized, err := NormalizeCandidate(item)
		if err != nil {
			continue
		}
		result = append(result, normalized)
	}
	return result
}

// MarshalJSON applies the same output boundary when a candidate is serialized
// by a handler or another package.
func (candidate Candidate) MarshalJSON() ([]byte, error) {
	normalized, err := NormalizeCandidate(candidate)
	if err != nil {
		return nil, err
	}
	type wire Candidate
	return json.Marshal(wire(normalized))
}

// UnmarshalJSON rejects candidates that would violate the provider-neutral
// response contract.  Provider payloads are parsed into private wire types in
// amap.go before this method is used.
func (candidate *Candidate) UnmarshalJSON(data []byte) error {
	if candidate == nil {
		return ErrInvalidRequest
	}
	type wire Candidate
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return ErrInvalidRequest
	}
	normalized, err := NormalizeCandidate(Candidate(decoded))
	if err != nil {
		return err
	}
	*candidate = normalized
	return nil
}

func cleanInput(value string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value))
}

func cleanField(value string) string {
	value = cleanInput(value)
	runes := []rune(value)
	if len(runes) > MaxCandidateFieldCodePoints {
		runes = runes[:MaxCandidateFieldCodePoints]
	}
	return string(runes)
}

func runeCount(value string) int {
	return len([]rune(value))
}

func validLatitude(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= -90 && value <= 90
}

func validLongitude(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= -180 && value <= 180
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
