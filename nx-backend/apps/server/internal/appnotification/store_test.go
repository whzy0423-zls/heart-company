package appnotification

import "testing"

func TestNormalizePage(t *testing.T) {
	tests := []struct {
		page, pageSize         int
		wantPage, wantPageSize int
	}{
		{0, 0, 1, 20},
		{-1, -1, 1, 20},
		{2, 50, 2, 50},
		{3, 101, 3, 20},
	}
	for _, tt := range tests {
		page, pageSize := normalizePage(tt.page, tt.pageSize)
		if page != tt.wantPage || pageSize != tt.wantPageSize {
			t.Fatalf("normalizePage(%d, %d)=(%d,%d), want (%d,%d)",
				tt.page, tt.pageSize, page, pageSize, tt.wantPage, tt.wantPageSize)
		}
	}
}
