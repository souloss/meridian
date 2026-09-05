package contracttest

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// TestStorageColumnsHaveComments keeps the executable migration schema and its
// documentation in lockstep. Every declared column must have one non-empty
// PostgreSQL COMMENT ON COLUMN declaration.
func TestStorageColumnsHaveComments(t *testing.T) {
	ddl, err := os.ReadFile(filepath.Join("..", "..", "migrations", "00001_m0_schema.sql"))
	if err != nil {
		t.Fatalf("read storage migration: %v", err)
	}

	tablePattern := regexp.MustCompile(`(?m)^CREATE TABLE ([a-z_][a-z0-9_]*) \($`)
	columnPattern := regexp.MustCompile(`^  ([a-z_][a-z0-9_]*)\s+`)
	columns := make(map[string]struct{})
	lines := strings.Split(string(ddl), "\n")
	var table string
	for _, line := range lines {
		if match := tablePattern.FindStringSubmatch(line); len(match) == 2 {
			table = match[1]
			continue
		}
		if table == "" {
			continue
		}
		if line == ");" {
			table = ""
			continue
		}
		if match := columnPattern.FindStringSubmatch(line); len(match) == 2 {
			columns[table+"."+match[1]] = struct{}{}
		}
	}
	if len(columns) == 0 {
		t.Fatal("storage migration does not declare table columns")
	}

	commentPattern := regexp.MustCompile(`(?m)^COMMENT ON COLUMN ([a-z_][a-z0-9_]*)\.([a-z_][a-z0-9_]*) IS '((?:''|[^'])*)';$`)
	comments := make(map[string]string)
	for _, match := range commentPattern.FindAllStringSubmatch(string(ddl), -1) {
		comments[match[1]+"."+match[2]] = strings.TrimSpace(match[3])
	}

	var missing []string
	for key := range columns {
		if strings.TrimSpace(comments[key]) == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		t.Fatalf("%d storage contract columns lack clear SQL comments:\n%s", len(missing), strings.Join(missing, "\n"))
	}
}
