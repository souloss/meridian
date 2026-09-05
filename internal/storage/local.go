// Package storage provides Meridian's local content-addressed blob store and content capabilities.
package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"sync"
)

const (
	digestBytes = sha256.Size
	digestChars = digestBytes * 2
)

// ErrBlobTooLarge indicates that a streamed object exceeded its caller-supplied limit.
var ErrBlobTooLarge = errors.New("blob exceeds the configured size limit")

// ErrBlobIntegrity indicates that an existing content-addressed target does not match its key.
var ErrBlobIntegrity = errors.New("blob content does not match its content-addressed key")

// ErrInvalidDigest indicates that a digest is not 64 lowercase hexadecimal characters.
var ErrInvalidDigest = errors.New("invalid SHA-256 blob digest")

// Blob describes one immutable object stored by its SHA-256 digest.
type Blob struct {
	// Digest is the lowercase hexadecimal SHA-256 digest of the complete content.
	Digest string
	// StorageKey is the slash-separated path relative to the configured root.
	StorageKey string
	// Size is the exact content length in bytes.
	Size int64
}

// LocalStore persists immutable blobs below one absolute filesystem root.
type LocalStore struct {
	root       string
	finalizeMu sync.Mutex
}

// NewLocalStore validates and initializes an absolute persistent-volume root.
func NewLocalStore(root string) (*LocalStore, error) {
	if !filepath.IsAbs(root) {
		return nil, errors.New("blob root must be an absolute path")
	}
	root = filepath.Clean(root)
	volume := filepath.VolumeName(root)
	if root == string(os.PathSeparator) || root == volume+string(os.PathSeparator) {
		return nil, errors.New("blob root cannot be a filesystem root")
	}
	objectsRoot := filepath.Join(root, "sha256")
	if err := os.MkdirAll(objectsRoot, 0o750); err != nil {
		return nil, fmt.Errorf("create blob root: %w", err)
	}
	return &LocalStore{root: root}, nil
}

// Root returns the normalized absolute persistent-volume root.
func (store *LocalStore) Root() string { return store.root }

// Put streams an object without an application size limit.
func (store *LocalStore) Put(ctx context.Context, source io.Reader) (Blob, error) {
	return store.put(ctx, source, -1)
}

// PutLimited streams an object and rejects content larger than maxBytes.
func (store *LocalStore) PutLimited(ctx context.Context, source io.Reader, maxBytes int64) (Blob, error) {
	if maxBytes < 0 {
		return Blob{}, errors.New("blob size limit cannot be negative")
	}
	if maxBytes == int64(^uint64(0)>>1) {
		return store.put(ctx, source, -1)
	}
	return store.put(ctx, source, maxBytes)
}

// Open opens a validated digest for streaming and returns its immutable metadata.
func (store *LocalStore) Open(digest string) (*os.File, Blob, error) {
	key, err := StorageKey(digest)
	if err != nil {
		return nil, Blob{}, err
	}
	file, err := os.Open(filepath.Join(store.root, filepath.FromSlash(key)))
	if err != nil {
		return nil, Blob{}, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, Blob{}, fmt.Errorf("stat blob: %w", err)
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, Blob{}, ErrBlobIntegrity
	}
	digestHash := sha256.New()
	if _, err := io.Copy(digestHash, file); err != nil {
		_ = file.Close()
		return nil, Blob{}, fmt.Errorf("verify opened blob: %w", err)
	}
	if hex.EncodeToString(digestHash.Sum(nil)) != digest {
		_ = file.Close()
		return nil, Blob{}, ErrBlobIntegrity
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		_ = file.Close()
		return nil, Blob{}, fmt.Errorf("rewind verified blob: %w", err)
	}
	return file, Blob{Digest: digest, StorageKey: key, Size: info.Size()}, nil
}

// StorageKey derives the only valid relative path for a lowercase SHA-256 digest.
func StorageKey(digest string) (string, error) {
	if !validDigest(digest) {
		return "", ErrInvalidDigest
	}
	return "sha256/" + digest[:2] + "/" + digest[2:4] + "/" + digest, nil
}

func (store *LocalStore) put(ctx context.Context, source io.Reader, maxBytes int64) (blob Blob, err error) {
	temporary, err := os.CreateTemp(filepath.Join(store.root, "sha256"), ".meridian-blob-*")
	if err != nil {
		return Blob{}, fmt.Errorf("create blob temporary file: %w", err)
	}
	temporaryName := temporary.Name()
	defer func() {
		_ = temporary.Close()
		if temporaryName != "" {
			_ = os.Remove(temporaryName)
		}
	}()

	digest := sha256.New()
	reader := io.Reader(&contextReader{ctx: ctx, reader: source})
	if maxBytes >= 0 {
		reader = io.LimitReader(reader, maxBytes+1)
	}
	size, err := io.Copy(io.MultiWriter(temporary, digest), reader)
	if err != nil {
		return Blob{}, fmt.Errorf("stream blob: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return Blob{}, err
	}
	if maxBytes >= 0 && size > maxBytes {
		return Blob{}, ErrBlobTooLarge
	}
	if err := temporary.Sync(); err != nil {
		return Blob{}, fmt.Errorf("sync blob temporary file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return Blob{}, fmt.Errorf("close blob temporary file: %w", err)
	}

	digestText := hex.EncodeToString(digest.Sum(nil))
	key, err := StorageKey(digestText)
	if err != nil {
		return Blob{}, err
	}
	blob = Blob{Digest: digestText, StorageKey: key, Size: size}
	target := filepath.Join(store.root, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return Blob{}, fmt.Errorf("create blob shard: %w", err)
	}

	store.finalizeMu.Lock()
	defer store.finalizeMu.Unlock()
	if info, statErr := os.Stat(target); statErr == nil {
		if err := verifyExistingBlob(target, info, blob, digest); err != nil {
			return Blob{}, err
		}
		return blob, nil
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return Blob{}, fmt.Errorf("stat content-addressed target: %w", statErr)
	}
	if err := os.Link(temporaryName, target); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return Blob{}, fmt.Errorf("commit content-addressed target: %w", err)
		}
		info, statErr := os.Stat(target)
		if statErr != nil {
			return Blob{}, fmt.Errorf("stat concurrently committed blob: %w", statErr)
		}
		if err := verifyExistingBlob(target, info, blob, digest); err != nil {
			return Blob{}, err
		}
		return blob, nil
	}
	if err := os.Remove(temporaryName); err != nil {
		return Blob{}, fmt.Errorf("remove committed blob temporary link: %w", err)
	}
	temporaryName = ""
	if err := syncDirectory(filepath.Dir(target)); err != nil {
		return Blob{}, err
	}
	return blob, nil
}

func verifyExistingBlob(path string, info os.FileInfo, expected Blob, reusableHash hash.Hash) error {
	if !info.Mode().IsRegular() || info.Size() != expected.Size {
		return ErrBlobIntegrity
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open existing blob: %w", err)
	}
	defer file.Close()
	reusableHash.Reset()
	if _, err := io.Copy(reusableHash, file); err != nil {
		return fmt.Errorf("verify existing blob: %w", err)
	}
	if hex.EncodeToString(reusableHash.Sum(nil)) != expected.Digest {
		return ErrBlobIntegrity
	}
	return nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open blob shard for sync: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync blob shard: %w", err)
	}
	return nil
}

func validDigest(digest string) bool {
	if len(digest) != digestChars {
		return false
	}
	for _, character := range digest {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader *contextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(buffer)
}
