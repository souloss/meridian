-- syncRepository 的幂等重放持久化查询。
-- 重放身份为 [tenantId, principalType, principalId, operationId, idempotencyKey]。

-- 在调用方事务内串行化一个 syncRepository 重放身份。
-- name: LockSyncIdempotency :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(lock_key), 0));

-- 返回 syncRepository 请求保留的准确响应。
-- name: GetSyncIdempotency :one
SELECT request_hash, response_body, expires_at
FROM idempotency_records
WHERE tenant_id = sqlc.arg(tenant_id)
  AND principal_type = sqlc.arg(principal_type)
  AND principal_id = sqlc.arg(principal_id)
  AND operation_id = 'syncRepository'
  AND idempotency_key = sqlc.arg(idempotency_key)
FOR UPDATE;

-- 在重新使用幂等键前删除已过期的 syncRepository 重放记录。
-- name: DeleteSyncIdempotency :exec
DELETE FROM idempotency_records
WHERE tenant_id = sqlc.arg(tenant_id)
  AND principal_type = sqlc.arg(principal_type)
  AND principal_id = sqlc.arg(principal_id)
  AND operation_id = 'syncRepository'
  AND idempotency_key = sqlc.arg(idempotency_key);

-- 保存可重放 24 小时的准确、非敏感 202 响应。
-- name: CreateSyncIdempotency :exec
INSERT INTO idempotency_records (
  tenant_id, principal_type, principal_id, operation_id, idempotency_key,
  request_hash, response_status, response_body, expires_at
) VALUES (
  sqlc.arg(tenant_id), sqlc.arg(principal_type), sqlc.arg(principal_id), 'syncRepository', sqlc.arg(idempotency_key),
  sqlc.arg(request_hash), 202, sqlc.arg(response_body)::jsonb, now() + interval '24 hours'
);
