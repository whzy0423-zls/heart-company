package skillcatalog

import "testing"

func TestLearningGrowthBuiltinCatalogHasSevenCategoriesAndThirtyFiveSkills(t *testing.T) {
	catalog := LearningGrowthBuiltinCatalog()
	if len(catalog.Categories) != 7 {
		t.Fatalf("categories=%d, want 7", len(catalog.Categories))
	}
	total, sourceNeeded := 0, 0
	seen := map[string]bool{}
	for _, category := range catalog.Categories {
		for _, skill := range category.Skills {
			total++
			if seen[skill.Key] {
				t.Fatalf("duplicate skill %q", skill.Key)
			}
			seen[skill.Key] = true
			if skill.SourceNeeded {
				sourceNeeded++
			}
		}
	}
	if total != 35 || sourceNeeded != 3 {
		t.Fatalf("total=%d sourceNeeded=%d, want 35/3", total, sourceNeeded)
	}
	for _, key := range []string{"deliberate-practice", "passing-from-your-world", "your-loneliness-is-glorious"} {
		for _, category := range catalog.Categories {
			for _, skill := range category.Skills {
				if skill.Key == key && !skill.SourceNeeded {
					t.Fatalf("source-needed skill %q must stay marked", key)
				}
			}
		}
	}
}
