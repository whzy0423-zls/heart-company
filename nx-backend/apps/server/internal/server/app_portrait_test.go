package server

import (
	"reflect"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/quiz"
)

func TestBuildPortraitIncludesPositiveGrowthSections(t *testing.T) {
	for mainType := 1; mainType <= 9; mainType++ {
		t.Run(quiz.TypeResults[mainType].Title, func(t *testing.T) {
			card := quiz.Card{MainType: mainType, UpdateTime: "2026/09/09 10:00:00"}

			got := buildPortrait(card)

			if !reflect.DeepEqual(got.Strengths, quiz.TypeResults[mainType].Strengths) {
				t.Fatalf("strengths = %#v, want %#v", got.Strengths, quiz.TypeResults[mainType].Strengths)
			}
			if len(got.PotentialDirections) == 0 {
				t.Fatal("potential directions should not be empty")
			}
			if len(got.SupportResources) == 0 {
				t.Fatal("support resources should not be empty")
			}
			if len(got.PositivePatterns) == 0 {
				t.Fatal("positive patterns should not be empty")
			}
			if len(got.StressSolutions) == 0 {
				t.Fatal("stress solutions should not be empty")
			}
			if len(got.EmotionSupport) == 0 {
				t.Fatal("emotion support should not be empty")
			}
		})
	}
}

func TestBuildPortraitWithoutAssessmentKeepsPositiveSectionsEmpty(t *testing.T) {
	got := buildPortrait(quiz.Card{})

	if got.Strengths != nil || got.PotentialDirections != nil || got.SupportResources != nil || got.PositivePatterns != nil || got.StressSolutions != nil || got.EmotionSupport != nil {
		t.Fatalf("positive sections should be empty without an assessment: %#v", got)
	}
}
