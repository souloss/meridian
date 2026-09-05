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
	paths, err := filepath.Glob(filepath.Join("..", "..", "migrations", "[0-9][0-9][0-9][0-9][0-9]_*.sql"))
	if err != nil {
		t.Fatalf("find storage migrations: %v", err)
	}
	slices.Sort(paths)
	if len(paths) == 0 {
		t.Fatal("storage migrations do not exist")
	}
	var upDDL strings.Builder
	for _, path := range paths {
		contents, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read storage migration %s: %v", path, readErr)
		}
		upDDL.WriteString(gooseUpSection(string(contents)))
		upDDL.WriteByte('\n')
	}
	ddl := upDDL.String()

	tablePattern := regexp.MustCompile(`(?m)^CREATE TABLE ([a-z_][a-z0-9_]*) \($`)
	columnPattern := regexp.MustCompile(`^  ([a-z_][a-z0-9_]*)\s+`)
	addColumnPattern := regexp.MustCompile(`(?m)^ALTER TABLE ([a-z_][a-z0-9_]*)\s*\nADD COLUMN ([a-z_][a-z0-9_]*)\s+`)
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
	for _, match := range addColumnPattern.FindAllStringSubmatch(ddl, -1) {
		columns[match[1]+"."+match[2]] = struct{}{}
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

func gooseUpSection(sql string) string {
	parts := strings.Split(sql, "-- +goose Down")
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}
