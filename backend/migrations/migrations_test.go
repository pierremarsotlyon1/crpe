package migrations

import (
	"io/fs"
	"testing"
)

func TestLatestTracksEmbeddedMigrations(t *testing.T) {
	names, err := fs.Glob(files, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) == 0 || names[len(names)-1] != Latest {
		t.Fatalf("Latest must match last embedded migration, got %v", names)
	}
}
