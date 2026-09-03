package appknowledge

import "testing"

func TestResolveBindingRowsKeepsTheoryAndOnlyCurrentType(t *testing.T) {
	rows := []bindingRow{
		{Layer: LayerTheory, LibraryID: 10, LibraryKey: "enneagram-core", LibraryStatus: "enabled", ReleaseID: 100, ReleaseStatus: "active"},
		{Layer: LayerEnneagramType, EnneagramType: intPointer(3), LibraryID: 13, LibraryKey: "enneagram-type-03", LibraryStatus: "enabled", ReleaseID: 103, ReleaseStatus: "active"},
		{Layer: LayerEnneagramType, EnneagramType: intPointer(2), LibraryID: 12, LibraryKey: "enneagram-type-02", LibraryStatus: "enabled", ReleaseID: 102, ReleaseStatus: "active"},
	}

	resolved := resolveBindingRows(3, rows)
	if resolved.Theory == nil || resolved.Theory.ReleaseID != 100 {
		t.Fatalf("missing theory binding: %+v", resolved)
	}
	if resolved.EnneagramType == nil || resolved.EnneagramType.ReleaseID != 103 || *resolved.EnneagramType.EnneagramType != 3 {
		t.Fatalf("wrong type binding: %+v", resolved)
	}
	if len(resolved.Diagnostics) != 1 || resolved.Diagnostics[0].Code != "cross_type_binding" {
		t.Fatalf("expected cross-type diagnostic, got %+v", resolved.Diagnostics)
	}
}

func TestResolveBindingRowsDegradesWithoutValidMainType(t *testing.T) {
	rows := []bindingRow{{Layer: LayerTheory, LibraryID: 10, LibraryKey: "enneagram-core", LibraryStatus: "enabled", ReleaseID: 100, ReleaseStatus: "active"}}
	for _, mainType := range []int{0, -1, 10} {
		resolved := resolveBindingRows(mainType, rows)
		if resolved.Theory == nil || resolved.EnneagramType != nil {
			t.Fatalf("mainType=%d should keep theory only: %+v", mainType, resolved)
		}
	}
}

func TestResolveBindingRowsRejectsDisabledMissingReleaseAndWrongLibrary(t *testing.T) {
	rows := []bindingRow{
		{Layer: LayerTheory, LibraryID: 10, LibraryKey: "enneagram-core", LibraryStatus: "disabled", ReleaseID: 100, ReleaseStatus: "active"},
		{Layer: LayerEnneagramType, EnneagramType: intPointer(3), LibraryID: 13, LibraryKey: "enneagram-type-02", LibraryStatus: "enabled", ReleaseID: 103, ReleaseStatus: "active"},
		{Layer: LayerEnneagramType, EnneagramType: intPointer(3), LibraryID: 14, LibraryKey: "enneagram-type-03", LibraryStatus: "enabled"},
	}

	resolved := resolveBindingRows(3, rows)
	if resolved.Theory != nil || resolved.EnneagramType != nil {
		t.Fatalf("invalid bindings must not resolve: %+v", resolved)
	}
	if len(resolved.Diagnostics) != 3 {
		t.Fatalf("expected three diagnostics, got %+v", resolved.Diagnostics)
	}
}

func intPointer(value int) *int { return &value }
