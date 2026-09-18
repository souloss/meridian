package repository

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"strings"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	generated "github.com/meridian-labs/meridian/internal/generated/repository"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/meridian-labs/meridian/internal/storage"
)

// AddBlobReference 原子地校验不可变元数据与租户的按字节去重配额。
func (store *RepositoryStore) AddBlobReference(ctx context.Context, tenantID uuid.UUID, blob storage.Blob, mediaType string, referencedAt time.Time) error {
	expectedKey, err := storage.StorageKey(blob.Digest)
	if err != nil || expectedKey != blob.StorageKey || blob.Size < 0 {
		return storage.ErrBlobMetadataConflict
	}
	parsedMediaType, _, err := mime.ParseMediaType(mediaType)
	if err != nil || parsedMediaType == "" || strings.ContainsAny(mediaType, "\r\n") {
		return storage.ErrBlobMetadataConflict
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin blob reference transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := generated.New(tx)
	limit, err := queries.LockTenantStorageQuota(ctx, tenantID)
	if err != nil {
		return normalizeError(err)
	}
	metadata, err := queries.CreateBlobMetadata(ctx, generated.CreateBlobMetadataParams{
		BlobDigest: blob.Digest, StorageKey: blob.StorageKey, SizeBytes: blob.Size, MediaType: mediaType,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		metadata, err = queries.GetBlobMetadata(ctx, blob.Digest)
	}
	if err != nil {
		return normalizeError(err)
	}
	if metadata.StorageKey != blob.StorageKey || metadata.SizeBytes != blob.Size || metadata.MediaType != mediaType {
		return storage.ErrBlobMetadataConflict
	}
	reference, err := queries.GetTenantBlobReference(ctx, generated.GetTenantBlobReferenceParams{TenantID: tenantID, BlobDigest: blob.Digest})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return normalizeError(err)
	}
	if errors.Is(err, pgx.ErrNoRows) || reference.RefCount == 0 {
		current, err := queries.CountTenantUniqueBlobBytes(ctx, tenantID)
		if err != nil {
			return normalizeError(err)
		}
		if current > limit-blob.Size {
			return &service.QuotaExceededError{Resource: quotaResourceStorageBytes, Current: current, Limit: limit}
		}
	}
	if _, err := queries.AddTenantBlobReference(ctx, generated.AddTenantBlobReferenceParams{
		TenantID: tenantID, BlobDigest: blob.Digest, ReferencedAt: timestamp(referencedAt),
	}); err != nil {
		return normalizeError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return normalizeError(err)
	}
	return nil
}

var _ storage.ReferenceRegistry = (*RepositoryStore)(nil)
