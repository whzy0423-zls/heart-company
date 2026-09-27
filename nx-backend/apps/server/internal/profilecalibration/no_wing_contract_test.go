package profilecalibration

import (
	"os"
	"strings"
	"testing"
)

func TestAcceptedReassessmentZerosRetiredWingColumn(t *testing.T) {
	raw, err := os.ReadFile("store.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "SET enneagram=$1, wing=0, profile=$2::jsonb") {
		t.Fatal("accepted reassessment must keep the retired card wing column at zero")
	}
}
