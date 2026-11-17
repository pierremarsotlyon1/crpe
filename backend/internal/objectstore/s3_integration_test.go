//go:build storageintegration

package objectstore

import (
	"bytes"
	"io"
	"os"
	"testing"
)

func TestS3ObjectLifecycle(t *testing.T) {
	if os.Getenv("S3_ENDPOINT") == "" {
		t.Fatal("S3_ENDPOINT required for storageintegration tests")
	}
	store, err := FromEnv(os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	key := "checks/" + t.Name() + ".txt"
	data := []byte("private test document")
	if err := store.Put(t.Context(), key, bytes.NewReader(data), int64(len(data)), "text/plain"); err != nil {
		t.Fatal(err)
	}
	reader, contentType, err := store.Get(t.Context(), key)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil || !bytes.Equal(body, data) || contentType != "text/plain" {
		t.Fatalf("object roundtrip: %q %q %v", body, contentType, err)
	}
	if err := store.Delete(t.Context(), key); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(t.Context(), key); err != nil {
		t.Fatalf("delete must be idempotent: %v", err)
	}
	reader, _, err = store.Get(t.Context(), key)
	if err == nil {
		_ = reader.Close()
		t.Fatal("deleted object still exists")
	}
}
