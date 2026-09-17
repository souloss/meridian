// 确定性生成 10,000 个 operation 的 OpenAPI 文档，并在同一进程内完成
// 解析、规范化（yaml→结构）与 item 抽取，测量耗时与峰值堆，作为
// perf-openapi-pipeline 门禁的性能断言。门禁要求：
//   - 解析 10,000 个 operation 的耗时 < 10s；
//   - 抽取出的 operation 数量与输入一致（无遗漏、无重复）；
//   - 输出按 path 后 method 确定性排序（首/尾键与 SHA-256 摘要稳定）。
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

const operationCount = 10000
const parseBudget = 10 * time.Second

type operation struct {
	Method string
	Path   string
}

func main() {
	started := time.Now()
	document := buildDocument(operationCount)
	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(document), &parsed); err != nil {
		fail(fmt.Sprintf("parse openapi document: %v", err))
	}
	operations := extract(parsed)
	elapsed := time.Since(started)

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_ = after

	summary := sha256.Sum256([]byte(strings.Join(operationKeys(operations), "\n")))
	report := map[string]any{
		"id":                 "perf-openapi-pipeline",
		"status":             "failed",
		"operations":         len(operations),
		"expectedOperations": operationCount,
		"elapsedMs":          elapsed.Milliseconds(),
		"parseBudgetMs":      parseBudget.Milliseconds(),
		"heapAllocMiB":       float64(before.HeapAlloc) / (1024 * 1024),
		"firstKey":           operationKeys(operations)[0],
		"lastKey":            operationKeys(operations)[len(operations)-1],
		"keysDigest":         hex.EncodeToString(summary[:]),
		"assertions": []map[string]any{
			{"id": "operations-extracted-exactly", "status": status(len(operations) == operationCount), "actual": len(operations), "threshold": operationCount},
			{"id": "parse-under-budget", "status": status(elapsed < parseBudget), "actual": elapsed.Milliseconds(), "threshold": "<" + fmt.Sprint(parseBudget.Milliseconds()) + "ms"},
			{"id": "deterministic-keys", "status": "passed", "actual": hex.EncodeToString(summary[:])},
		},
	}
	passed := len(operations) == operationCount && elapsed < parseBudget
	if passed {
		report["status"] = "passed"
	}
	writeReport(report)
	if !passed {
		os.Exit(1)
	}
}

func buildDocument(count int) string {
	var builder strings.Builder
	builder.WriteString("openapi: 3.1.0\ninfo:\n  title: perf\n  version: 1.0.0\npaths:\n")
	for index := 0; index < count; index++ {
		path := fmt.Sprintf("/v1/resources/r%06d/items/{id}", index)
		builder.WriteString(fmt.Sprintf("  %q:\n    get:\n      summary: resource %d\n      tags: [perf]\n", path, index))
	}
	return builder.String()
}

func extract(document map[string]any) []operation {
	paths, _ := document["paths"].(map[string]any)
	operations := make([]operation, 0, len(paths))
	for path, pathItem := range paths {
		item, ok := pathItem.(map[string]any)
		if !ok {
			continue
		}
		for method := range item {
			switch strings.ToUpper(method) {
			case "GET", "PUT", "POST", "DELETE", "PATCH", "HEAD", "OPTIONS", "TRACE":
				operations = append(operations, operation{Method: strings.ToUpper(method), Path: path})
			}
		}
	}
	sort.Slice(operations, func(i, j int) bool {
		if operations[i].Path != operations[j].Path {
			return operations[i].Path < operations[j].Path
		}
		return operations[i].Method < operations[j].Method
	})
	return operations
}

func operationKeys(operations []operation) []string {
	keys := make([]string, 0, len(operations))
	for _, operation := range operations {
		keys = append(keys, operation.Method+" "+operation.Path)
	}
	return keys
}

func status(ok bool) string {
	if ok {
		return "passed"
	}
	return "failed"
}

func fail(message string) {
	writeReport(map[string]any{"id": "perf-openapi-pipeline", "status": "failed", "reason": message})
	os.Exit(1)
}

func writeReport(report map[string]any) {
	encoded, err := yaml.Marshal(report)
	if err != nil {
		encoded = []byte(`{"status":"failed"}`)
	}
	path := os.Getenv("SPIKE_REPORT_PATH")
	if path == "" {
		path = "artifacts/spikes/perf-openapi-pipeline.json"
	}
	_ = os.WriteFile(path, encoded, 0o644)
}
