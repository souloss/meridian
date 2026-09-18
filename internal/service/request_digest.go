package service

import (
	"crypto/sha256"
	"encoding/json"
	"sort"
)

// RequestDigest 对一次幂等请求的 RFC 8785 JCS 规范对象计算 SHA-256 请求摘要。对象键按升序
// 输出（encoding/json 已按排序键序列化 map）；数字使用默认紧凑编码，对 M1 幂等操作承载的
// string、UUID 与 boolean 值是确定性的。
//
// 规范对象匹配 contracts/domain.yaml 的 idempotency.requestDigest：operationId、pathParameters、
// query、semanticHeaders（存在 if-match 时）与解析后的 JSON body（缺失时为 nil）。
func RequestDigest(operationID string, pathParameters map[string]any, semanticHeaders map[string]any, body any) ([]byte, error) {
	canonical := map[string]any{
		"operationId":     operationID,
		"pathParameters":  sortedValue(pathParameters),
		"query":           []any{},
		"semanticHeaders": sortedValue(semanticHeaders),
		"body":            body,
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(encoded)
	return sum[:], nil
}

// sortedValue 对 nil map 返回空 map，否则返回 map 本身；encoding/json 按排序键序列化 map，
// 这正是所需的 JCS 键排序。
func sortedValue(value map[string]any) any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

// SortedQueryPairs 将查询串的百分号解码名/值对规范化为请求摘要所需的确定性顺序。M1
// 幂等操作不携带查询参数，因此它恒返回空列表；保留为契约定义的接缝。
func SortedQueryPairs() []any {
	return []any{}
}

var _ = sort.Strings
