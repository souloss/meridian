// Command godoc completes documentation on generated API and repository code.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type insertion struct {
	offset int
	text   string
}

type generatedKind string

const (
	apiKind        generatedKind = "api"
	repositoryKind generatedKind = "repository"
)

func main() {
	directory := flag.String("dir", "internal/generated/api", "directory containing generated Go files")
	flag.Parse()
	if err := documentDirectory(*directory); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func documentDirectory(directory string) error {
	files, err := filepath.Glob(filepath.Join(directory, "*.go"))
	if err != nil {
		return fmt.Errorf("find generated files: %w", err)
	}
	if len(files) == 0 {
		return errors.New("no generated Go files found")
	}
	kind := apiKind
	if filepath.Base(directory) == "repository" {
		kind = repositoryKind
	}
	for _, filename := range files {
		if strings.HasSuffix(filename, "_test.go") {
			continue
		}
		if err := documentFile(filename, kind); err != nil {
			return err
		}
	}
	return nil
}

func documentFile(filename string, kind generatedKind) error {
	source, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("read %s: %w", filename, err)
	}
	// Upstream generators still emit the pre-Go 1.18 spelling in a few stable
	// interfaces. The generated surface targets Go 1.27 exclusively.
	source = bytes.ReplaceAll(source, []byte("interface{}"), []byte("any"))
	files := token.NewFileSet()
	parsed, err := parser.ParseFile(files, filename, source, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parse %s: %w", filename, err)
	}

	var insertions []insertion
	for _, declaration := range parsed.Decls {
		collectDeclarationComments(files, source, declaration, kind, &insertions)
	}
	if len(insertions) == 0 {
		return nil
	}
	slices.SortFunc(insertions, func(left, right insertion) int { return right.offset - left.offset })
	for _, item := range insertions {
		source = slices.Concat(source[:item.offset], []byte(item.text), source[item.offset:])
	}
	formatted, err := format.Source(source)
	if err != nil {
		return fmt.Errorf("format documented %s: %w", filename, err)
	}
	if err := os.WriteFile(filename, formatted, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", filename, err)
	}
	return nil
}

func collectDeclarationComments(files *token.FileSet, source []byte, declaration ast.Decl, kind generatedKind, insertions *[]insertion) {
	switch declaration := declaration.(type) {
	case *ast.FuncDecl:
		if declaration.Name.IsExported() && !validComment(declaration.Name.Name, declaration.Doc) {
			position := declaration.Pos()
			if declaration.Doc != nil {
				position = declaration.Doc.Pos()
			}
			addLineComment(files, source, position, functionComment(declaration.Name.Name, kind), insertions)
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
					if !validComment(specification.Name.Name, documentation) {
						position := declaration.Pos()
						if documentation != nil {
							position = documentation.Pos()
						}
						addLineComment(files, source, position, typeComment(specification.Name.Name, kind), insertions)
					}
				}
				collectFieldComments(files, source, specification.Name.Name, specification.Type, kind, insertions)
			case *ast.ValueSpec:
				for _, name := range specification.Names {
					if name.IsExported() && declaration.Doc == nil && specification.Doc == nil && specification.Comment == nil {
						addLineComment(files, source, declaration.Pos(), valueComment(name.Name), insertions)
						break
					}
				}
			}
		}
	}
}

func collectFieldComments(files *token.FileSet, source []byte, owner string, expression ast.Expr, kind generatedKind, insertions *[]insertion) {
	ast.Inspect(expression, func(node ast.Node) bool {
		field, ok := node.(*ast.Field)
		if !ok {
			return true
		}
		for _, name := range field.Names {
			documentation := field.Doc
			if documentation == nil {
				documentation = field.Comment
			}
			if name.IsExported() && !validComment(name.Name, documentation) {
				position := field.Pos()
				if documentation != nil {
					position = documentation.Pos()
				}
				addLineComment(files, source, position, fieldComment(owner, name.Name, kind), insertions)
				break
			}
		}
		return true
	})
}

func validComment(name string, documentation *ast.CommentGroup) bool {
	if documentation == nil {
		return false
	}
	comment := strings.TrimSpace(documentation.Text())
	return strings.HasPrefix(comment, name+" ") && !strings.HasPrefix(comment, name+" "+name+" ")
}

func addLineComment(files *token.FileSet, source []byte, position token.Pos, comment string, insertions *[]insertion) {
	offset := files.PositionFor(position, false).Offset
	lineStart := bytes.LastIndexByte(source[:offset], '\n') + 1
	indentation := string(source[lineStart:offset])
	if strings.TrimSpace(indentation) == "" {
		*insertions = append(*insertions, insertion{offset: offset, text: "// " + comment + "\n" + indentation})
		return
	}
	*insertions = append(*insertions, insertion{offset: offset, text: "/* " + comment + " */ "})
}

func typeComment(name string, kind generatedKind) string {
	if kind == repositoryKind {
		switch {
		case name == "DBTX":
			return "DBTX is the pgx query and transaction surface required by generated repository methods."
		case name == "Queries":
			return "Queries executes the strongly typed SQL statements generated from migrations/queries."
		case name == "Querier":
			return "Querier exposes every generated Meridian database query for dependency injection and tests."
		case strings.HasSuffix(name, "Params"):
			return name + " contains the strongly typed arguments for the " + strings.TrimSuffix(name, "Params") + " query."
		case strings.HasSuffix(name, "Row"):
			return name + " contains the columns returned by the " + strings.TrimSuffix(name, "Row") + " query."
		default:
			return name + " is the generated PostgreSQL representation of the corresponding Meridian table row."
		}
	}
	switch {
	case name == "HttpRequestDoer":
		return "HttpRequestDoer sends generated client requests and returns HTTP responses."
	case name == "ClientInterface" || name == "ClientWithResponsesInterface":
		return name + " exposes every client operation generated from the OpenAPI contract."
	case strings.HasSuffix(name, "RequestObject"):
		return name + " contains validated inputs for its OpenAPI operation."
	case strings.HasSuffix(name, "ResponseObject"):
		return name + " is implemented by every declared response for its OpenAPI operation."
	case strings.HasSuffix(name, "ResponseHeaders") || strings.HasSuffix(name, "Headers"):
		return name + " contains headers declared by the corresponding OpenAPI response."
	case strings.HasSuffix(name, "Response"):
		return name + " contains the raw HTTP response and any decoded response body."
	case strings.HasSuffix(name, "Params"):
		return name + " contains validated path, query, header, or cookie parameters."
	default:
		return name + " is generated transport code derived from the Meridian OpenAPI contract."
	}
}

func functionComment(name string, kind generatedKind) string {
	if kind == repositoryKind {
		switch name {
		case "New":
			return "New binds generated repository queries to a pgx pool or transaction."
		case "WithTx":
			return "WithTx returns generated repository queries bound to the supplied pgx transaction."
		default:
			return name + " executes the generated " + name + " database query."
		}
	}
	return name + " implements generated transport behavior for the Meridian OpenAPI contract."
}

func valueComment(name string) string {
	return name + " is generated from the Meridian OpenAPI contract."
}

func fieldComment(owner, name string, kind generatedKind) string {
	if kind == repositoryKind {
		if owner == "Querier" || owner == "DBTX" {
			return name + " exposes the corresponding strongly typed database operation."
		}
		query := strings.TrimSuffix(strings.TrimSuffix(owner, "Params"), "Row")
		if strings.HasSuffix(owner, "Row") {
			return name + " is the " + splitIdentifier(name) + " value returned by the " + query + " query."
		}
		if strings.HasSuffix(owner, "Params") {
			return name + " is the " + splitIdentifier(name) + " value supplied to the " + query + " query."
		}
		return name + " is the generated " + splitIdentifier(name) + " database value for " + owner + "."
	}
	switch name {
	case "Do":
		return "Do sends one HTTP request and returns its response."
	case "Server":
		return "Server is the base URL used for generated client requests."
	case "Client":
		return "Client performs generated HTTP requests."
	case "RequestEditors":
		return "RequestEditors mutate each generated request before it is sent."
	case "Body":
		return "Body contains the decoded or raw HTTP response body."
	case "HTTPResponse":
		return "HTTPResponse is the underlying response returned by net/http."
	case "Params":
		return "Params contains the validated parameters for this request."
	case "Headers":
		return "Headers contains the headers declared for this response."
	case "StatusCode":
		return "StatusCode is the HTTP status code for a variable-status response."
	case "ContentType":
		return "ContentType is the media type of the request or response body."
	case "ContentLength":
		return "ContentLength is the byte length of an unparsed response body."
	case "XRequestId":
		return "XRequestId correlates the response with server logs and audit records."
	case "ETag":
		return "ETag is the entity tag used for optimistic concurrency control."
	default:
		if strings.HasSuffix(owner, "Interface") {
			return name + " handles the corresponding operation from the Meridian OpenAPI contract."
		}
		return name + " carries the generated " + splitIdentifier(name) + " value for " + owner + "."
	}
}

func splitIdentifier(value string) string {
	var result strings.Builder
	characters := []rune(value)
	for index, character := range characters {
		isUpper := character >= 'A' && character <= 'Z'
		previousLower := index > 0 && characters[index-1] >= 'a' && characters[index-1] <= 'z'
		nextLower := index+1 < len(characters) && characters[index+1] >= 'a' && characters[index+1] <= 'z'
		if index > 0 && isUpper && (previousLower || nextLower) {
			result.WriteByte(' ')
		}
		result.WriteRune(character)
	}
	return strings.ToLower(result.String())
}
