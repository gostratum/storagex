package storagex

import "testing"

func TestNoOpKeyBuilder(t *testing.T) {
	kb := &NoOpKeyBuilder{}
	if got := kb.BuildKey("a/b", nil); got != "a/b" {
		t.Fatalf("expected %s, got %s", "a/b", got)
	}
}

func TestPrefixKeyBuilder(t *testing.T) {
	kb := NewPrefixKeyBuilder("base")
	if got := kb.BuildKey("file.txt", nil); got != "base/file.txt" {
		t.Fatalf("expected %s, got %s", "base/file.txt", got)
	}
}
