package chatappearance

import "testing"

func TestStoreExists(t *testing.T) {
	if NewStore(nil) == nil {
		t.Fatal("store should be constructible")
	}
}
