package db

import "testing"

func TestStoryManagementButtonsIncludeCRUDPermissions(t *testing.T) {
	want := map[string]bool{
		"App:StoryManagement:Edit":    false,
		"App:StoryManagement:Delete":  false,
		"App:StoryManagement:Publish": false,
	}
	for _, button := range storyManagementButtons {
		if _, ok := want[button.code]; ok {
			want[button.code] = true
		}
	}
	for code, found := range want {
		if !found {
			t.Errorf("missing story management permission %s", code)
		}
	}
}

func TestStoryManagementUsesAvailableBookIcon(t *testing.T) {
	if storyManagementIcon != "lucide:book-open-text" {
		t.Fatalf("story management icon=%q", storyManagementIcon)
	}
}
