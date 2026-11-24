package runner

import (
	"path/filepath"
	"testing"
)

func TestStorageAt(t *testing.T) {
	root := t.TempDir()
	storage := storageAt(root)
	if storage == nil || !storage.Valid() {
		t.Fatalf("invalid filesystem capacity: %+v", storage)
	}
	if storageAt(filepath.Join(root, "missing")) != nil {
		t.Fatal("missing path reported storage")
	}
}
