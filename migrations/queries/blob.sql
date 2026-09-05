-- LockTenantStorageQuota serializes all tenant blob-reference accounting and
-- returns the frozen unique-byte quota copied into the tenant snapshot.
-- name: LockTenantStorageQuota :one
SELECT COALESCE((quota ->> 'maxStorageBytes')::bigint, 0::bigint)::bigint AS limit_bytes
FROM tenants
WHERE id = sqlc.arg(tenant_id)
  AND status = 'active'
FOR UPDATE;

-- CreateBlobMetadata inserts immutable content-addressed metadata and returns
-- no row when another tenant or request already registered the same digest.
-- name: CreateBlobMetadata :one
INSERT INTO blobs (blob_digest, storage_key, size_bytes, media_type)
VALUES (sqlc.arg(blob_digest), sqlc.arg(storage_key), sqlc.arg(size_bytes), sqlc.arg(media_type))
ON CONFLICT (blob_digest) DO NOTHING
RETURNING *;

-- GetBlobMetadata returns immutable metadata for validating a reused digest.
-- name: GetBlobMetadata :one
SELECT *
FROM blobs
WHERE blob_digest = sqlc.arg(blob_digest);

-- GetTenantBlobReference returns the current count after the caller locks the tenant quota row.
-- name: GetTenantBlobReference :one
SELECT *
FROM tenant_blob_refs
WHERE tenant_id = sqlc.arg(tenant_id)
  AND blob_digest = sqlc.arg(blob_digest);

-- CountTenantUniqueBlobBytes sums each positively referenced global blob once
-- so repeated revisions of identical content do not consume quota again.
-- name: CountTenantUniqueBlobBytes :one
SELECT COALESCE(SUM(blobs.size_bytes), 0::numeric)::bigint AS current_bytes
FROM tenant_blob_refs
JOIN blobs ON blobs.blob_digest = tenant_blob_refs.blob_digest
WHERE tenant_blob_refs.tenant_id = sqlc.arg(tenant_id)
  AND tenant_blob_refs.ref_count > 0;

-- AddTenantBlobReference creates or increments one tenant reference after the
-- caller has serialized and validated unique-byte quota accounting.
-- name: AddTenantBlobReference :one
INSERT INTO tenant_blob_refs (tenant_id, blob_digest, ref_count, last_referenced_at)
VALUES (sqlc.arg(tenant_id), sqlc.arg(blob_digest), 1, sqlc.arg(referenced_at)::timestamptz)
ON CONFLICT (tenant_id, blob_digest) DO UPDATE
SET ref_count = tenant_blob_refs.ref_count + 1,
    last_referenced_at = EXCLUDED.last_referenced_at
RETURNING *;
