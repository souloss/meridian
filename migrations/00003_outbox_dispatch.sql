-- 为待发送和租约过期的 outbox 记录增加调度扫描索引，匹配 SKIP LOCKED 查询顺序。
-- +goose Up
CREATE INDEX notify_outbox_due_idx
ON notify_outbox (next_attempt_at, created_at, tenant_id, id)
WHERE status IN ('pending', 'failed', 'delivering');

COMMENT ON INDEX notify_outbox_due_idx IS
  '按 next_attempt_at、创建时间、租户和记录标识排序，供 SKIP LOCKED 调度器有界扫描可投递及租约过期记录。';

-- 删除 outbox 调度索引；回滚后扫描会退化为更高成本的路径。
-- +goose Down
DROP INDEX IF EXISTS notify_outbox_due_idx;
