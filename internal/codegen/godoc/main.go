// Command godoc 为生成的 API 与仓储代码补齐文档注释。
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
	// 上游生成器在若干稳定接口中仍输出 Go 1.18 之前的拼写。
	// 生成的表面代码仅面向 Go 1.27。
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
			return "DBTX 是生成仓储方法所需的 pgx 查询与事务接口。"
		case name == "Queries":
			return "Queries 执行从 migrations/queries 生成的强类型 SQL 语句。"
		case name == "Querier":
			return "Querier 暴露每一个生成的 Meridian 数据库查询，供依赖注入与测试使用。"
		case strings.HasSuffix(name, "Params"):
			return name + " 包含 " + strings.TrimSuffix(name, "Params") + " 查询的强类型参数。"
		case strings.HasSuffix(name, "Row"):
			return name + " 包含 " + strings.TrimSuffix(name, "Row") + " 查询返回的列。"
		default:
			return name + " 是相应 Meridian 表行的生成 PostgreSQL 表示。"
		}
	}
	switch {
	case name == "HttpRequestDoer":
		return "HttpRequestDoer 发送生成的客户端请求并返回 HTTP 响应。"
	case name == "ClientInterface" || name == "ClientWithResponsesInterface":
		return name + " 暴露从 OpenAPI 契约生成的每一个客户端操作。"
	case strings.HasSuffix(name, "RequestObject"):
		return name + " 包含其 OpenAPI 操作的已校验输入。"
	case strings.HasSuffix(name, "ResponseObject"):
		return name + " 由其 OpenAPI 操作声明的每一个响应实现。"
	case strings.HasSuffix(name, "ResponseHeaders") || strings.HasSuffix(name, "Headers"):
		return name + " 包含对应 OpenAPI 响应声明的头部。"
	case strings.HasSuffix(name, "Response"):
		return name + " 包含原始 HTTP 响应与解码后的响应体。"
	case strings.HasSuffix(name, "Params"):
		return name + " 包含已校验的路径、查询、头部或 Cookie 参数。"
	default:
		return name + " 是从 Meridian OpenAPI 契约派生的生成传输代码。"
	}
}

func functionComment(name string, kind generatedKind) string {
	if kind == repositoryKind {
		switch name {
		case "New":
			return "New 将生成的仓储查询绑定到 pgx 连接池或事务。"
		case "WithTx":
			return "WithTx 返回绑定到所提供 pgx 事务的生成仓储查询。"
		default:
			return name + " 执行生成的 " + name + " 数据库查询。"
		}
	}
	return name + " 实现 Meridian OpenAPI 契约的生成传输行为。"
}

func valueComment(name string) string {
	return name + " 从 Meridian OpenAPI 契约生成。"
}

func fieldComment(owner, name string, kind generatedKind) string {
	if kind == repositoryKind {
		if owner == "Querier" || owner == "DBTX" {
			return name + " 暴露相应的强类型数据库操作。"
		}
		query := strings.TrimSuffix(strings.TrimSuffix(owner, "Params"), "Row")
		if strings.HasSuffix(owner, "Row") {
			return name + " 是 " + query + " 查询返回的 " + name + " 值。"
		}
		if strings.HasSuffix(owner, "Params") {
			return name + " 是提供给 " + query + " 查询的 " + name + " 值。"
		}
		return name + " 是 " + owner + " 的生成 " + name + " 数据库值。"
	}
	switch name {
	case "Do":
		return "Do 发送一次 HTTP 请求并返回其响应。"
	case "Server":
		return "Server 是生成客户端请求使用的基础 URL。"
	case "Client":
		return "Client 执行生成的 HTTP 请求。"
	case "RequestEditors":
		return "RequestEditors 在请求发送前修改每一个生成的请求。"
	case "Body":
		return "Body 包含解码或原始的 HTTP 响应体。"
	case "HTTPResponse":
		return "HTTPResponse 是 net/http 返回的底层响应。"
	case "Params":
		return "Params 包含此请求的已校验参数。"
	case "Headers":
		return "Headers 包含为此响应声明的头部。"
	case "StatusCode":
		return "StatusCode 是变状态响应的 HTTP 状态码。"
	case "ContentType":
		return "ContentType 是请求或响应体的媒体类型。"
	case "ContentLength":
		return "ContentLength 是未解析响应体的字节长度。"
	case "XRequestId":
		return "XRequestId 将响应与服务端日志及审计记录关联。"
	case "ETag":
		return "ETag 是用于乐观并发控制的实体标签。"
	default:
		if strings.HasSuffix(owner, "Interface") {
			return name + " 处理 Meridian OpenAPI 契约中的对应操作。"
		}
		return name + " 承载 " + owner + " 的生成 " + name + " 值。"
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
