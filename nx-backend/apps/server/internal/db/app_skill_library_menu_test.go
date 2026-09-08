package db

import "testing"

func TestSkillLibraryManagementMenuBelongsToAppManagement(t *testing.T) {
	if skillLibraryManagementPath != "/app/skill-library" {
		t.Fatalf("path=%q", skillLibraryManagementPath)
	}
	if skillLibraryManagementViewPermission != "App:SkillLibrary:View" {
		t.Fatalf("view permission=%q", skillLibraryManagementViewPermission)
	}
	if skillLibraryManagementEditPermission != "App:SkillLibrary:Edit" {
		t.Fatalf("edit permission=%q", skillLibraryManagementEditPermission)
	}
}
