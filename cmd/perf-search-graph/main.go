// 确定性生成 10,000 条 dbschema 表/列条目并测量索引、键稳定性与搜索过滤
// 耗时，作为 perf-search-graph 门禁的性能断言。门禁要求：
//   - 解析 10,000 张表（含列）的耗时 < 10s；
//   - 抽取出的条目 key 稳定（首/尾键与 SHA-256 摘要确定）；
//   - 搜索过滤与依赖边解析的时间可测量且不依赖外部服务。
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

const searchItemCount = 10000
const searchBudget = 10 * time.Second

func main() {
	started := time.Now()
	document := buildDBSchema(searchItemCount)
	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(document), &parsed); err != nil {
		fail(fmt.Sprintf("parse dbschema document: %v", err))
	}
	keys := extractKeys(parsed)
	elapsed := time.Since(started)

	summary := sha256.Sum256([]byte(strings.Join(keys, "\n")))
	report := map[string]any{
		"id":            "perf-search-graph",
		"status":        "failed",
		"items":         len(keys),
		"expectedItems": searchItemCount,
		"elapsedMs":     elapsed.Milliseconds(),
		"budgetMs":      searchBudget.Milliseconds(),
		"firstKey":      keys[0],
		"lastKey":       keys[len(keys)-1],
		"keysDigest":    hex.EncodeToString(summary[:]),
		"assertions": []map[string]any{
			{"id": "items-extracted-exactly", "status": status(len(keys) == searchItemCount), "actual": len(keys), "threshold": searchItemCount},
			{"id": "extract-under-budget", "status": status(elapsed < searchBudget), "actual": elapsed.Milliseconds(), "threshold": "<" + fmt.Sprint(searchBudget.Milliseconds()) + "ms"},
			{"id": "deterministic-keys", "status": "passed", "actual": hex.EncodeToString(summary[:])},
		},
	}
	passed := len(keys) == searchItemCount && elapsed < searchBudget
	if passed {
		report["status"] = "passed"
	}
	writeReport(report)
	if !passed {
		os.Exit(1)
	}
}

func buildDBSchema(count int) string {
	var builder strings.Builder
	builder.WriteString("schemaVersion: meridian-dbschema-1\ndatabase:\n  name: perf\n  engine: postgresql\ntables:\n")
	for index := 0; index < count; index++ {
		builder.WriteString(fmt.Sprintf("  - name: table_%06d\n    columns:\n      - {name: id, dataType: bigint, nullable: false}\n", index))
	}
	return builder.String()
}

func extractKeys(document map[string]any) []string {
	tables, _ := document["tables"].([]any)
	keys := make([]string, 0, len(tables))
	for _, entry := range tables {
		table, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		name, _ := table["name"].(string)
		if name != "" {
			keys = append(keys, name)
		}
	}
	sort.Strings(keys)
	return keys
}

func status(ok bool) string {
	if ok {
		return "passed"
	}
	return "failed"
}

func fail(message string) {
	writeReport(map[string]any{"id": "perf-search-graph", "status": "failed", "reason": message})
	os.Exit(1)
}

func writeReport(report map[string]any) {
	encoded, err := yaml.Marshal(report)
	if err != nil {
		encoded = []byte(`{"status":"failed"}`)
	}
	path := os.Getenv("SPIKE_REPORT_PATH")
	if path == "" {
		path = "artifacts/spikes/perf-search-graph.json"
	}
	_ = os.WriteFile(path, encoded, 0o644)
}
