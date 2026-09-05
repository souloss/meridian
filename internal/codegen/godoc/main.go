// Command godoc completes documentation on oapi-codegen transport plumbing.
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

func main() {
	directory := flag.String("dir", "internal/generated/api", "directory containing generated Go files")
	flag.Parse()
	if err := documentDirectory(*directory); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func documentDirectory(directory string) error {
	files, err := filepath.Glob(filepath.Join(directory, "*.gen.go"))
	if err != nil {
		return fmt.Errorf("find generated files: %w", err)
	}
	if len(files) == 0 {
		return errors.New("no generated Go files found")
	}
	for _, filename := range files {
		if err := documentFile(filename); err != nil {
			return err
		}
	}
	return nil
}

func documentFile(filename string) error {
	source, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("read %s: %w", filename, err)
	}
	files := token.NewFileSet()
	parsed, err := parser.ParseFile(files, filename, source, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parse %s: %w", filename, err)
	}

	var insertions []insertion
	for _, declaration := range parsed.Decls {
		collectDeclarationComments(files, source, declaration, &insertions)
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

func collectDeclarationComments(files *token.FileSet, source []byte, declaration ast.Decl, insertions *[]insertion) {
	switch declaration := declaration.(type) {
	case *ast.FuncDecl:
		if declaration.Name.IsExported() && !validComment(declaration.Name.Name, declaration.Doc) {
			position := declaration.Pos()
			if declaration.Doc != nil {
				position = declaration.Doc.Pos()
			}
			addLineComment(files, source, position, functionComment(declaration.Name.Name), insertions)
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
						addLineComment(files, source, position, typeComment(specification.Name.Name), insertions)
					}
				}
				collectFieldComments(files, source, specification.Name.Name, specification.Type, insertions)
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

func collectFieldComments(files *token.FileSet, source []byte, owner string, expression ast.Expr, insertions *[]insertion) {
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
				addLineComment(files, source, position, fieldComment(owner, name.Name), insertions)
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

func typeComment(name string) string {
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

func functionComment(name string) string {
	return name + " implements generated transport behavior for the Meridian OpenAPI contract."
}

func valueComment(name string) string {
	return name + " is generated from the Meridian OpenAPI contract."
}

func fieldComment(owner, name string) string {
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
	for index, character := range value {
		if index > 0 && character >= 'A' && character <= 'Z' {
			result.WriteByte(' ')
		}
		result.WriteRune(character)
	}
	return strings.ToLower(result.String())
}
