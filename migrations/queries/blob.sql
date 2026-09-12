-- 串行化租户对象引用的全部配额核算，并返回复制到租户快照中的固定字节配额。
-- name: LockTenantStorageQuota :one
SELECT COALESCE((quota ->> 'maxStorageBytes')::bigint, 0::bigint)::bigint AS limit_bytes
FROM tenants
WHERE id = sqlc.arg(tenant_id)
  AND status = 'active'
FOR UPDATE;

-- 写入不可变的内容寻址元数据；其他租户或请求已登记相同摘要时不返回记录。
-- name: CreateBlobMetadata :one
INSERT INTO blobs (blob_digest, storage_key, size_bytes, media_type)
VALUES (sqlc.arg(blob_digest), sqlc.arg(storage_key), sqlc.arg(size_bytes), sqlc.arg(media_type))
ON CONFLICT (blob_digest) DO NOTHING
RETURNING *;

-- 返回不可变对象元数据，用于校验重复使用的摘要。
-- name: GetBlobMetadata :one
SELECT *
FROM blobs
WHERE blob_digest = sqlc.arg(blob_digest);

-- 调用方锁定租户配额行后，返回当前对象引用数。
-- name: GetTenantBlobReference :one
SELECT *
FROM tenant_blob_refs
WHERE tenant_id = sqlc.arg(tenant_id)
  AND blob_digest = sqlc.arg(blob_digest);

-- 每个有正引用的全局对象只计入一次，避免相同内容的重复版本重复消耗配额。
-- name: CountTenantUniqueBlobBytes :one
SELECT COALESCE(SUM(blobs.size_bytes), 0::numeric)::bigint AS current_bytes
FROM tenant_blob_refs
JOIN blobs ON blobs.blob_digest = tenant_blob_refs.blob_digest
WHERE tenant_blob_refs.tenant_id = sqlc.arg(tenant_id)
  AND tenant_blob_refs.ref_count > 0;

-- 调用方完成串行化并校验唯一字节配额后，创建或递增一条租户对象引用。
-- name: AddTenantBlobReference :one
INSERT INTO tenant_blob_refs (tenant_id, blob_digest, ref_count, last_referenced_at)
VALUES (sqlc.arg(tenant_id), sqlc.arg(blob_digest), 1, sqlc.arg(referenced_at)::timestamptz)
ON CONFLICT (tenant_id, blob_digest) DO UPDATE
SET ref_count = tenant_blob_refs.ref_count + 1,
    last_referenced_at = EXCLUDED.last_referenced_at
RETURNING *;
