package server

import "testing"

func TestDefaultAppFeatureConfigKeepsLifeStoryVisible(t *testing.T) {
	if !defaultAppFeatureConfig().LifeStoryEnabled {
		t.Fatal("life story should remain visible by default")
	}
}
