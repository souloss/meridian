package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestLocalStoreWritesDeduplicatesAndOpensCASBlob(t *testing.T) {
	t.Parallel()
	store, err := NewLocalStore(filepath.Join(t.TempDir(), "objects"))
	if err != nil {
		t.Fatalf("construct local store: %v", err)
	}
	content := []byte("meridian immutable artifact\n")
	first, err := store.Put(t.Context(), bytes.NewReader(content))
	if err != nil {
		t.Fatalf("put first blob: %v", err)
	}
	second, err := store.Put(t.Context(), bytes.NewReader(content))
	if err != nil {
		t.Fatalf("put duplicate blob: %v", err)
	}
	if first != second || first.StorageKey != "sha256/98/97/98972c16dfc5c4684ed27d7e01a38dc3cc27afa2862b044229e45c36b96ac157" || first.Size != int64(len(content)) {
		t.Fatalf("blob metadata = %#v/%#v", first, second)
	}
	file, opened, err := store.Open(first.Digest)
	if err != nil {
		t.Fatalf("open blob: %v", err)
	}
	defer file.Close()
	got, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("read blob: %v", err)
	}
	if !bytes.Equal(got, content) || opened != first {
		t.Fatalf("opened blob = %#v/%q, want %#v/%q", opened, got, first, content)
	}
	entries, err := os.ReadDir(filepath.Join(store.Root(), "sha256"))
	if err != nil {
		t.Fatalf("list object root: %v", err)
	}
	for _, entry := range entries {
		if len(entry.Name()) > 0 && entry.Name()[0] == '.' {
			t.Fatalf("temporary file remained after commit: %s", entry.Name())
		}
	}
}

func TestLocalStoreRejectsOversizeCancellationAndUnsafeInputs(t *testing.T) {
	t.Parallel()
	if _, err := NewLocalStore("relative/blob-root"); err == nil {
		t.Fatal("relative blob root unexpectedly accepted")
	}
	if _, err := NewLocalStore(string(os.PathSeparator)); err == nil {
		t.Fatal("filesystem root unexpectedly accepted")
	}
	store, err := NewLocalStore(filepath.Join(t.TempDir(), "objects"))
	if err != nil {
		t.Fatalf("construct local store: %v", err)
	}
	if _, err := store.PutLimited(t.Context(), bytes.NewReader([]byte("12345")), 4); !errors.Is(err, ErrBlobTooLarge) {
		t.Fatalf("oversize error = %v, want ErrBlobTooLarge", err)
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.Put(cancelled, bytes.NewReader([]byte("content"))); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled put error = %v, want context.Canceled", err)
	}
	for _, digest := range []string{"", "../artifact", "ABCDEF", string(bytes.Repeat([]byte{'g'}, digestChars))} {
		if _, err := StorageKey(digest); !errors.Is(err, ErrInvalidDigest) {
			t.Errorf("StorageKey(%q) error = %v, want ErrInvalidDigest", digest, err)
		}
	}
}

func TestLocalStoreDetectsCorruptExistingTarget(t *testing.T) {
	t.Parallel()
	store, err := NewLocalStore(filepath.Join(t.TempDir(), "objects"))
	if err != nil {
		t.Fatalf("construct local store: %v", err)
	}
	content := []byte("expected-content")
	blob, err := store.Put(t.Context(), bytes.NewReader(content))
	if err != nil {
		t.Fatalf("put blob: %v", err)
	}
	target := filepath.Join(store.Root(), filepath.FromSlash(blob.StorageKey))
	if err := os.WriteFile(target, bytes.Repeat([]byte{'x'}, len(content)), 0o600); err != nil {
		t.Fatalf("corrupt blob fixture: %v", err)
	}
	if file, _, err := store.Open(blob.Digest); !errors.Is(err, ErrBlobIntegrity) {
		if file != nil {
			_ = file.Close()
		}
		t.Fatalf("open corrupt target error = %v, want ErrBlobIntegrity", err)
	}
	if _, err := store.Put(t.Context(), bytes.NewReader(content)); !errors.Is(err, ErrBlobIntegrity) {
		t.Fatalf("corrupt target error = %v, want ErrBlobIntegrity", err)
	}
}

func TestLocalStoreSerializesConcurrentFinalization(t *testing.T) {
	t.Parallel()
	store, err := NewLocalStore(filepath.Join(t.TempDir(), "objects"))
	if err != nil {
		t.Fatalf("construct local store: %v", err)
	}
	content := bytes.Repeat([]byte("concurrent-meridian"), 1024)
	results := make([]Blob, 8)
	errorsByWriter := make([]error, len(results))
	var writers sync.WaitGroup
	for index := range results {
		writers.Go(func() {
			results[index], errorsByWriter[index] = store.Put(t.Context(), bytes.NewReader(content))
		})
	}
	writers.Wait()
	for index, err := range errorsByWriter {
		if err != nil || results[index] != results[0] {
			t.Fatalf("writer %d result/error = %#v/%v, want %#v/nil", index, results[index], err, results[0])
		}
	}
}
