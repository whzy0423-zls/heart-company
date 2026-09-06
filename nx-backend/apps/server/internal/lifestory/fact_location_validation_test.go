package lifestory

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateFactCardLocationsAcceptsNewValuesAtMost120Runes(t *testing.T) {
	value := strings.Repeat("地", maxFactLocationCodePoints)
	facts := FactCard{Events: []FactEvent{{ID: "event-1", Location: value}}}
	if err := validateFactCardLocations(facts, FactCard{}); err != nil {
		t.Fatalf("120-rune location rejected: %v", err)
	}

	tooLong := strings.Repeat("地", maxFactLocationCodePoints+1)
	err := validateFactCardLocations(
		FactCard{Events: []FactEvent{{ID: "event-1", Location: tooLong}}},
		FactCard{},
	)
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("new 121-rune location error=%v, want ErrValidation", err)
	}
}

func TestValidateFactCardLocationsAllowsUnchangedHistoricalLongValueByID(t *testing.T) {
	legacy := strings.Repeat("旧", maxFactLocationCodePoints+40)
	current := FactCard{Events: []FactEvent{{ID: "event-1", Location: legacy}}}
	incoming := FactCard{Events: []FactEvent{{ID: "event-1", Location: legacy, Description: "补充描述"}}}
	if err := validateFactCardLocations(incoming, current); err != nil {
		t.Fatalf("unchanged historical value rejected: %v", err)
	}

	changed := FactCard{Events: []FactEvent{{ID: "event-1", Location: legacy + "改"}}}
	if err := validateFactCardLocations(changed, current); !errors.Is(err, ErrValidation) {
		t.Fatalf("changed historical value error=%v, want ErrValidation", err)
	}
}

func TestValidateFactCardLocationsUsesIndexWhenLegacyEventHasNoID(t *testing.T) {
	legacy := strings.Repeat("历", maxFactLocationCodePoints+1)
	current := FactCard{Events: []FactEvent{{Location: legacy}}}
	incoming := FactCard{Events: []FactEvent{{Location: legacy, Description: "其他字段变化"}}}
	if err := validateFactCardLocations(incoming, current); err != nil {
		t.Fatalf("unchanged no-ID legacy value rejected: %v", err)
	}

	misaligned := FactCard{Events: []FactEvent{{Location: "另一地点"}, {Location: legacy}}}
	if err := validateFactCardLocations(misaligned, current); !errors.Is(err, ErrValidation) {
		t.Fatalf("index-misaligned legacy value error=%v, want ErrValidation", err)
	}
}

func TestValidateFactCardLocationsChecksEventsAndTimelineIndependently(t *testing.T) {
	legacy := strings.Repeat("时", maxFactLocationCodePoints+1)
	current := FactCard{
		Events:   []FactEvent{{ID: "event-1", Location: legacy}},
		Timeline: []FactEvent{{ID: "timeline-1", Location: legacy}},
	}
	incoming := current
	incoming.Timeline = []FactEvent{{ID: "timeline-1", Location: legacy + "改"}}
	if err := validateFactCardLocations(incoming, current); !errors.Is(err, ErrValidation) {
		t.Fatalf("timeline change error=%v, want ErrValidation", err)
	}
}

func TestValidateFactCardLocationsRejectsLongValueWhenEventIDDoesNotMatch(t *testing.T) {
	legacy := strings.Repeat("位", maxFactLocationCodePoints+1)
	current := FactCard{Events: []FactEvent{{ID: "old", Location: legacy}}}
	incoming := FactCard{Events: []FactEvent{{ID: "new", Location: legacy}}}
	if err := validateFactCardLocations(incoming, current); !errors.Is(err, ErrValidation) {
		t.Fatalf("unmatched event ID error=%v, want ErrValidation", err)
	}
}

func TestValidateFactCardLocationsCoordinates(t *testing.T) {
	lat, lng := 31.2304, 121.4737
	valid := FactCard{Events: []FactEvent{{Latitude: &lat, Longitude: &lng, CoordinateSystem: "gcj02"}}}
	if err := validateFactCardLocations(valid, FactCard{}); err != nil {
		t.Fatalf("valid coordinates rejected: %v", err)
	}
	badLatitude := 91.0
	invalid := FactCard{Events: []FactEvent{{Latitude: &badLatitude, Longitude: &lng}}}
	if err := validateFactCardLocations(invalid, FactCard{}); err == nil {
		t.Fatal("out-of-range latitude accepted")
	}
	partial := FactCard{Events: []FactEvent{{Latitude: &lat}}}
	if err := validateFactCardLocations(partial, FactCard{}); err == nil {
		t.Fatal("partial coordinates accepted")
	}
}
