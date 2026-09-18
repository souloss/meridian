// Package storage 提供 Meridian 的本地内容寻址 blob 存储与内容能力。
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

// 存储布局与校验常量。
const (
	// digestBytes 是 SHA-256 摘要的原始字节长度。
	digestBytes = sha256.Size
	// digestChars 是十六进制编码后的摘要字符长度。
	digestChars = digestBytes * 2
	// blobShardDirectory 是相对根目录的摘要分片子目录名。
	blobShardDirectory = "sha256"
	// blobDirectoryPermission 是创建 blob 目录时使用的 Unix 权限位。
	blobDirectoryPermission = 0o750
	// unlimitedBlobSize 表示不受应用侧大小限制的写入哨兵值。
	unlimitedBlobSize = -1
)

// ErrBlobTooLarge 表示流式写入的对象超出了调用方提供的上限。
var ErrBlobTooLarge = errors.New("blob exceeds the configured size limit")

// ErrBlobIntegrity 表示已存在的内容寻址目标与其键不匹配。
var ErrBlobIntegrity = errors.New("blob content does not match its content-addressed key")

// ErrInvalidDigest 表示摘要不是 64 个小写十六进制字符。
var ErrInvalidDigest = errors.New("invalid SHA-256 blob digest")

// Blob 描述一个按其 SHA-256 摘要存储的不可变对象。
type Blob struct {
	// Digest 是完整内容的小写十六进制 SHA-256 摘要。
	Digest string
	// StorageKey 是相对于已配置根目录的斜杠分隔路径。
	StorageKey string
	// Size 是内容的精确字节长度。
	Size int64
}

// LocalStore 在一个绝对文件系统根目录下持久化不可变 blob。
type LocalStore struct {
	root       string
	finalizeMu sync.Mutex
}

// NewLocalStore 校验并初始化一个绝对持久卷根目录。
func NewLocalStore(root string) (*LocalStore, error) {
	if !filepath.IsAbs(root) {
		return nil, errors.New("blob root must be an absolute path")
	}
	root = filepath.Clean(root)
	volume := filepath.VolumeName(root)
	if root == string(os.PathSeparator) || root == volume+string(os.PathSeparator) {
		return nil, errors.New("blob root cannot be a filesystem root")
	}
	objectsRoot := filepath.Join(root, blobShardDirectory)
	if err := os.MkdirAll(objectsRoot, blobDirectoryPermission); err != nil {
		return nil, fmt.Errorf("create blob root: %w", err)
	}
	return &LocalStore{root: root}, nil
}

// Root 返回规范化的绝对持久卷根目录。
func (store *LocalStore) Root() string { return store.root }

// Put 以不施加应用侧大小限制的方式流式写入一个对象。
func (store *LocalStore) Put(ctx context.Context, source io.Reader) (Blob, error) {
	return store.put(ctx, source, unlimitedBlobSize)
}

// PutLimited 流式写入一个对象并拒绝超过 maxBytes 的内容。
func (store *LocalStore) PutLimited(ctx context.Context, source io.Reader, maxBytes int64) (Blob, error) {
	if maxBytes < 0 {
		return Blob{}, errors.New("blob size limit cannot be negative")
	}
	if maxBytes == int64(^uint64(0)>>1) {
		return store.put(ctx, source, unlimitedBlobSize)
	}
	return store.put(ctx, source, maxBytes)
}

// Open 打开一个经过校验的摘要用于流式读取并返回其不可变元数据。
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

// StorageKey 为一个小写 SHA-256 摘要推导出唯一合法的相对路径。
func StorageKey(digest string) (string, error) {
	if !validDigest(digest) {
		return "", ErrInvalidDigest
	}
	return blobShardDirectory + "/" + digest[:2] + "/" + digest[2:4] + "/" + digest, nil
}

// put 将源内容流式写入临时文件，校验后提交为内容寻址目标。
func (store *LocalStore) put(ctx context.Context, source io.Reader, maxBytes int64) (blob Blob, err error) {
	temporary, err := os.CreateTemp(filepath.Join(store.root, blobShardDirectory), ".meridian-blob-*")
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
	if err := os.MkdirAll(filepath.Dir(target), blobDirectoryPermission); err != nil {
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

// verifyExistingBlob 校验已存在的目标是否与期望的摘要及大小一致。
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

// syncDirectory 将目录内容同步到磁盘以持久化刚提交的硬链接。
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

// validDigest 判断一个字符串是否为合法的 64 位小写十六进制 SHA-256 摘要。
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

// contextReader 在每次读取前检查上下文取消，以便及时中止流式写入。
type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

// Read 在底层读取前检查上下文是否已取消。
func (reader *contextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(buffer)
}
