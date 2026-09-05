package contracttest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

const maximumReportedDocumentationFailures = 50

func TestOpenAPIDocumentsGeneratedSurface(t *testing.T) {
	document := readOpenAPI(t)
	var missing []string

	paths := mapping(t, document, "paths")
	for path, pathValue := range paths {
		pathItem := asMapping(t, pathValue, "paths."+path)
		for method, operationValue := range pathItem {
			if !slices.Contains([]string{"delete", "get", "head", "options", "patch", "post", "put", "trace"}, method) {
				continue
			}
			operation := asMapping(t, operationValue, "paths."+path+"."+method)
			requireText(&missing, operation, "description", "operation "+strings.ToUpper(method)+" "+path)
			checkParameters(t, &missing, operation["parameters"], "operation "+strings.ToUpper(method)+" "+path)
		}
		checkParameters(t, &missing, pathItem["parameters"], "path "+path)
	}

	components := mapping(t, document, "components")
	schemas := mapping(t, components, "schemas")
	for name, schemaValue := range schemas {
		schema := asMapping(t, schemaValue, "components.schemas."+name)
		requireText(&missing, schema, "description", "schema "+name)
		checkSchemaProperties(t, &missing, schema, "schema "+name)
	}
	checkParameters(t, &missing, components["parameters"], "component parameters")

	assertNoMissingDocumentation(t, missing)
}

func TestGeneratedGoExportsHaveComments(t *testing.T) {
	var generatedFiles []string
	for _, directory := range []string{"api", "repository"} {
		matches, err := filepath.Glob(filepath.Join("..", "generated", directory, "*.go"))
		if err != nil {
			t.Fatalf("find generated %s Go files: %v", directory, err)
		}
		generatedFiles = append(generatedFiles, matches...)
	}
	if len(generatedFiles) == 0 {
		t.Fatal("no generated Go files found")
	}

	var missing []string
	files := token.NewFileSet()
	for _, generatedFile := range generatedFiles {
		parsed, err := parser.ParseFile(files, generatedFile, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse %s: %v", generatedFile, err)
		}
		for _, declaration := range parsed.Decls {
			checkDeclarationComments(&missing, filepath.Base(generatedFile), declaration)
		}
	}
	assertNoMissingDocumentation(t, missing)
}

func readOpenAPI(t *testing.T) map[string]any {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "..", "contracts", "openapi.yaml"))
	if err != nil {
		t.Fatalf("read OpenAPI contract: %v", err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(content, &document); err != nil {
		t.Fatalf("parse OpenAPI contract: %v", err)
	}
	return document
}

func checkParameters(t *testing.T, missing *[]string, value any, owner string) {
	t.Helper()
	if value == nil {
		return
	}
	parameters, ok := value.([]any)
	if !ok {
		parameterMap, mapOK := value.(map[string]any)
		if !mapOK {
			t.Fatalf("%s parameters has type %T", owner, value)
		}
		for name, parameterValue := range parameterMap {
			parameter := asMapping(t, parameterValue, owner+" parameter "+name)
			requireText(missing, parameter, "description", owner+" parameter "+name)
		}
		return
	}
	for _, parameterValue := range parameters {
		parameter := asMapping(t, parameterValue, owner+" parameter")
		if _, reference := parameter["$ref"]; reference {
			continue
		}
		name, _ := parameter["name"].(string)
		if name == "" {
			name = "inline"
		}
		requireText(missing, parameter, "description", owner+" parameter "+name)
	}
}

func checkSchemaProperties(t *testing.T, missing *[]string, schema map[string]any, owner string) {
	t.Helper()
	if propertiesValue, exists := schema["properties"]; exists {
		properties := asMapping(t, propertiesValue, owner+" properties")
		for name, propertyValue := range properties {
			property := asMapping(t, propertyValue, owner+" property "+name)
			requireText(missing, property, "description", owner+" property "+name)
			checkSchemaProperties(t, missing, property, owner+" property "+name)
		}
	}
	for _, composition := range []string{"allOf", "anyOf", "oneOf"} {
		branches, exists := schema[composition].([]any)
		if !exists {
			continue
		}
		for _, branchValue := range branches {
			branch := asMapping(t, branchValue, owner+" "+composition)
			checkSchemaProperties(t, missing, branch, owner+" "+composition+" branch")
		}
	}
	if itemsValue, exists := schema["items"]; exists {
		items := asMapping(t, itemsValue, owner+" items")
		checkSchemaProperties(t, missing, items, owner+" item")
	}
}

func checkDeclarationComments(missing *[]string, filename string, declaration ast.Decl) {
	switch declaration := declaration.(type) {
	case *ast.FuncDecl:
		if declaration.Name.IsExported() {
			checkGoComment(missing, filename+" function", declaration.Name.Name, declaration.Doc)
		}
	case *ast.GenDecl:
		for _, specification := range declaration.Specs {
			switch specification := specification.(type) {
			case *ast.TypeSpec:
				if specification.Name.IsExported() {
					documentation := specification.Doc
					if documentation == nil {
						documentation = declaration.Doc
					}
					checkGoComment(missing, filename+" type", specification.Name.Name, documentation)
				}
				checkGeneratedFieldComments(missing, filename, specification.Name.Name, specification.Type)
			case *ast.ValueSpec:
				for _, name := range specification.Names {
					if name.IsExported() && declaration.Doc == nil && specification.Doc == nil && specification.Comment == nil {
						*missing = append(*missing, filename+" value "+name.Name)
					}
				}
			}
		}
	}
}

func checkGeneratedFieldComments(missing *[]string, filename, owner string, expression ast.Expr) {
	ast.Inspect(expression, func(node ast.Node) bool {
		field, ok := node.(*ast.Field)
		if !ok {
			return true
		}
		for _, name := range field.Names {
			if name.IsExported() {
				documentation := field.Doc
				if documentation == nil {
					documentation = field.Comment
				}
				checkGoComment(missing, filename+" field "+owner, name.Name, documentation)
			}
		}
		return true
	})
}

func checkGoComment(missing *[]string, owner, name string, documentation *ast.CommentGroup) {
	if documentation == nil {
		*missing = append(*missing, owner+" "+name)
		return
	}
	comment := strings.TrimSpace(documentation.Text())
	if !strings.HasPrefix(comment, name+" ") || strings.HasPrefix(comment, name+" "+name+" ") {
		*missing = append(*missing, owner+" "+name+" malformed comment: "+comment)
	}
}

func mapping(t *testing.T, value map[string]any, key string) map[string]any {
	t.Helper()
	return asMapping(t, value[key], key)
}

func asMapping(t *testing.T, value any, path string) map[string]any {
	t.Helper()
	mapping, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s has type %T, want mapping", path, value)
	}
	return mapping
}

func requireText(missing *[]string, value map[string]any, key, owner string) {
	text, ok := value[key].(string)
	if !ok || strings.TrimSpace(text) == "" {
		*missing = append(*missing, owner+" "+key)
	}
}

func assertNoMissingDocumentation(t *testing.T, missing []string) {
	t.Helper()
	if len(missing) == 0 {
		return
	}
	slices.Sort(missing)
	reported := missing
	if len(reported) > maximumReportedDocumentationFailures {
		reported = reported[:maximumReportedDocumentationFailures]
	}
	t.Fatalf("%d contract or generated surfaces lack clear source documentation (showing %d):\n%s", len(missing), len(reported), strings.Join(reported, "\n"))
}
