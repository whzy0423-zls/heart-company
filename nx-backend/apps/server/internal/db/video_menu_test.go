package db

import "testing"

func TestVideoMenuOnlyExposesInfiniteCanvas(t *testing.T) {
	var videoMenus []seedMenu
	for _, menu := range defaultMenus {
		if menu.ID == 1000 || menu.PID == 1000 {
			videoMenus = append(videoMenus, menu)
		}
	}

	if len(videoMenus) != 2 {
		t.Fatalf("video menu count = %d, want catalog plus Infinite Canvas only: %+v", len(videoMenus), videoMenus)
	}

	catalog, canvas := videoMenus[0], videoMenus[1]
	if catalog.ID != 1000 || catalog.Path != "/video" || catalog.Title != "视频生成" || catalog.Type != "catalog" {
		t.Fatalf("unexpected video catalog: %+v", catalog)
	}
	if canvas.ID != 1001 || canvas.PID != 1000 || canvas.Name != "InfiniteCanvas" {
		t.Fatalf("unexpected Infinite Canvas menu identity: %+v", canvas)
	}
	if canvas.Path != "/video/infinite-canvas" || canvas.Component != "/video/infinite-canvas" {
		t.Fatalf("unexpected Infinite Canvas route: %+v", canvas)
	}
	if canvas.Title != "无限画布" || canvas.Icon != "lucide:workflow" {
		t.Fatalf("unexpected Infinite Canvas metadata: %+v", canvas)
	}
}

func TestDeprecatedMenusRemoveLegacyVideoChildren(t *testing.T) {
	for _, menuID := range []string{"1002", "1003", "1004", "1005", "1006", "1007", "1008", "1010"} {
		if !containsSQLID(deprecatedMenusSQL, menuID) {
			t.Fatalf("deprecated menu cleanup must remove legacy video menu %s", menuID)
		}
	}
}

func containsSQLID(sql, id string) bool {
	return len(sql) > 0 && (contains(sql, "id = "+id) || contains(sql, "id IN (1002,1003,1004,1005,1006,1007,1008,1010)"))
}

func contains(value, fragment string) bool {
	for index := 0; index+len(fragment) <= len(value); index++ {
		if value[index:index+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
