// These guards inspect same-package production structure; they do not claim
// compile-time isolation between files in the httpapi package.
package httpapi

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const (
	productClientImport = "github.com/erranliu/SteerLine/internal/product/client"
	sqliteImport        = "github.com/erranliu/SteerLine/internal/storage/sqlite"
)

type httpAPISourceFile struct {
	name    string
	file    *ast.File
	imports map[string]string
}

type httpAPITypeDefinition struct {
	expression ast.Expr
	imports    map[string]string
}

func TestHTTPCommandBoundaryArchitecture(t *testing.T) {
	sources, err := loadHTTPAPIProductionSources(".")
	if err != nil {
		t.Fatal(err)
	}
	if violations := httpAPIArchitectureViolations(sources); len(violations) != 0 {
		t.Fatalf("HTTP command boundary architecture violations:\n%s", strings.Join(violations, "\n"))
	}
}

func TestHTTPCommandBoundaryArchitectureRejectsPrivateFixtures(t *testing.T) {
	for _, fixture := range []struct {
		name       string
		sources    map[string]string
		wantGuards []string
	}{
		{
			name: "server_cannot_bind_concrete_product_client",
			sources: map[string]string{"endpoint.go": `package httpapi
import productclient "github.com/erranliu/SteerLine/internal/product/client"
type Server struct { product *productclient.Client }
`},
			wantGuards: []string{"production httpapi imports concrete product client"},
		},
		{
			name: "registry_cannot_capture_transport_client_or_database_handles",
			sources: map[string]string{"command_registry.go": `package httpapi
import (
  "net/http"
  "net"
  "os"
  "database/sql"
  productclient "github.com/erranliu/SteerLine/internal/product/client"
)
type Server struct{}
type commandRegistry struct {
  server *Server
  request *http.Request
  writer http.ResponseWriter
  transport *http.Client
  product *productclient.Client
  db *sql.DB
  connection *sql.Conn
  network net.Conn
  file *os.File
}
func newCommandRegistry(server *Server, request *http.Request, writer http.ResponseWriter, transport *http.Client, product *productclient.Client, db *sql.DB, connection *sql.Conn, network net.Conn, file *os.File) *commandRegistry { return nil }
`},
			wantGuards: []string{
				"commandRegistry boundary forbidden type: Server",
				"commandRegistry boundary forbidden type: net/http.Request",
				"commandRegistry boundary forbidden type: net/http.ResponseWriter",
				"commandRegistry boundary forbidden type: net/http.Client",
				"commandRegistry boundary forbidden type: product/client.Client",
				"commandRegistry boundary forbidden type: database/sql.DB",
				"commandRegistry boundary forbidden type: database/sql.Conn",
				"commandRegistry boundary forbidden type: net.Conn",
				"commandRegistry boundary forbidden type: os.File",
			},
		},
		{
			name: "registry_cannot_open_a_second_record_path",
			sources: map[string]string{"command_registry.go": `package httpapi
import "os"
type requestRecord struct{}
type requestRecordAccess interface { Read(string) (requestRecord, error) }
type commandRegistry struct { records requestRecordAccess }
func (c *commandRegistry) readDirect(path string) { _, _ = os.ReadFile(path); _, _ = readRequestRecord(path) }
`},
			wantGuards: []string{"command_registry.go bypasses the request record owner"},
		},
		{
			name: "record_access_has_one_production_owner",
			sources: map[string]string{
				"request_records.go": `package httpapi
type requestRecord struct{}
type requestRecordAccess interface { Read(string) (requestRecord, error) }
type fileRequestRecordOwner struct{}
func (o *fileRequestRecordOwner) Read(string) (requestRecord, error) { return requestRecord{}, nil }
`,
				"duplicate.go": `package httpapi
type duplicateRecordOwner struct{}
func (o *duplicateRecordOwner) Read(string) (requestRecord, error) { return requestRecord{}, nil }
`,
			},
			wantGuards: []string{"request record reader is defined outside fileRequestRecordOwner"},
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			sources, err := parseHTTPAPISources(fixture.sources)
			if err != nil {
				t.Fatal(err)
			}
			violations := httpAPIArchitectureViolations(sources)
			for _, expected := range fixture.wantGuards {
				if !containsViolation(violations, expected) {
					t.Errorf("fixture escaped its expected guard %q: %v", expected, violations)
				}
			}
		})
	}
}

func loadHTTPAPIProductionSources(dir string) ([]httpAPISourceFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	contents := make(map[string]string)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		contents[name] = string(body)
	}
	return parseHTTPAPISources(contents)
}

func parseHTTPAPISources(contents map[string]string) ([]httpAPISourceFile, error) {
	names := make([]string, 0, len(contents))
	for name := range contents {
		names = append(names, name)
	}
	sort.Strings(names)
	files := make([]httpAPISourceFile, 0, len(names))
	for _, name := range names {
		file, err := parser.ParseFile(token.NewFileSet(), name, contents[name], parser.AllErrors)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		imports, err := sourceImports(file)
		if err != nil {
			return nil, fmt.Errorf("imports in %s: %w", name, err)
		}
		files = append(files, httpAPISourceFile{name: name, file: file, imports: imports})
	}
	return files, nil
}

func sourceImports(file *ast.File) (map[string]string, error) {
	imports := make(map[string]string, len(file.Imports))
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return nil, err
		}
		name := filepath.Base(path)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		imports[name] = path
	}
	return imports, nil
}

func httpAPIArchitectureViolations(sources []httpAPISourceFile) []string {
	var violations []string
	typeDefs := make(map[string]httpAPITypeDefinition)
	structs := make(map[string][]struct {
		file   httpAPISourceFile
		fields *ast.StructType
	})
	interfaceCounts := make(map[string]int)
	methodCounts := make(map[string]int)
	ownerTypeCount, recordTypeCount := 0, 0
	constructorCount, constructorCalls := 0, 0
	for _, source := range sources {
		for alias, path := range source.imports {
			if path == productClientImport {
				violations = append(violations, "production httpapi imports concrete product client in "+source.name)
			}
			if (path == "database/sql" || path == sqliteImport || path == "path/filepath") && source.name == "command_registry.go" {
				violations = append(violations, "command_registry.go imports a concrete storage or filesystem package: "+path)
			}
			if (path == "net/http" || path == "database/sql" || path == sqliteImport) && (alias == "." || alias == "_") && source.name == "command_registry.go" {
				violations = append(violations, "command_registry.go dot-imports a broad transport or storage package")
			}
		}
		for _, decl := range source.file.Decls {
			switch declaration := decl.(type) {
			case *ast.GenDecl:
				if declaration.Tok != token.TYPE {
					continue
				}
				for _, spec := range declaration.Specs {
					typeSpec := spec.(*ast.TypeSpec)
					typeDefs[typeSpec.Name.Name] = httpAPITypeDefinition{expression: typeSpec.Type, imports: source.imports}
					if typeSpec.Name.Name == "fileRequestRecordOwner" {
						ownerTypeCount++
						if source.name != "request_records.go" {
							violations = append(violations, "fileRequestRecordOwner is defined outside request_records.go")
						}
					}
					if typeSpec.Name.Name == "requestRecord" {
						recordTypeCount++
						if source.name != "request_records.go" {
							violations = append(violations, "requestRecord format is defined outside request_records.go")
						}
					}
					if structType, ok := typeSpec.Type.(*ast.StructType); ok {
						structs[typeSpec.Name.Name] = append(structs[typeSpec.Name.Name], struct {
							file   httpAPISourceFile
							fields *ast.StructType
						}{file: source, fields: structType})
					}
					if typeSpec.Name.Name == "requestRecordAccess" {
						interfaceCounts[typeSpec.Name.Name]++
						if !isRequestRecordAccessInterface(typeSpec.Type) {
							violations = append(violations, "requestRecordAccess must expose only Read, Register, and Write")
						}
					}
				}
			case *ast.FuncDecl:
				if declaration.Name.Name == "newCommandRegistry" {
					constructorCount++
					if !hasNamedParameterType(declaration.Type.Params, "records", "requestRecordAccess") {
						violations = append(violations, "newCommandRegistry must receive requestRecordAccess")
					}
				}
				if declaration.Name.Name == "newFileRequestRecordOwner" {
					constructorCalls++ // corrected below to count declaration and calls separately
				}
				if receiverTypeName(declaration) == "fileRequestRecordOwner" && isRecordAccessMethod(declaration.Name.Name) {
					methodCounts[declaration.Name.Name]++
				}
				if isRecordAccessMethod(declaration.Name.Name) && functionMentionsRequestRecord(declaration) && receiverTypeName(declaration) != "fileRequestRecordOwner" {
					violations = append(violations, "request record reader is defined outside fileRequestRecordOwner: "+declaration.Name.Name)
				}
			}
		}
	}
	if constructorCount != 1 {
		violations = append(violations, fmt.Sprintf("newCommandRegistry definition count is %d, want 1", constructorCount))
	}
	if interfaceCounts["requestRecordAccess"] != 1 {
		violations = append(violations, fmt.Sprintf("requestRecordAccess definition count is %d, want 1", interfaceCounts["requestRecordAccess"]))
	}
	if ownerTypeCount != 1 {
		violations = append(violations, fmt.Sprintf("fileRequestRecordOwner definition count is %d, want 1", ownerTypeCount))
	}
	if recordTypeCount != 1 {
		violations = append(violations, fmt.Sprintf("requestRecord definition count is %d, want 1", recordTypeCount))
	}
	for _, method := range []string{"Read", "Register", "Write"} {
		if methodCounts[method] != 1 {
			violations = append(violations, fmt.Sprintf("fileRequestRecordOwner.%s definition count is %d, want 1", method, methodCounts[method]))
		}
	}

	if len(structs["Server"]) != 1 {
		violations = append(violations, fmt.Sprintf("Server struct definition count is %d, want 1", len(structs["Server"])))
	}
	for _, owner := range structs["Server"] {
		for _, field := range owner.fields.Fields.List {
			for _, dependency := range forbiddenTypeDependencies(field.Type, owner.file.imports, typeDefs, false, make(map[string]bool)) {
				violations = append(violations, "Server has forbidden product or storage dependency: "+dependency)
			}
		}
	}
	if len(structs["commandRegistry"]) != 1 {
		violations = append(violations, fmt.Sprintf("commandRegistry struct definition count is %d, want 1", len(structs["commandRegistry"])))
	}
	for _, owner := range structs["commandRegistry"] {
		recordStoreFields := 0
		for _, field := range owner.fields.Fields.List {
			if fieldName(field) == "records" && isIdentType(field.Type, "requestRecordAccess") {
				recordStoreFields++
			}
			for _, dependency := range forbiddenTypeDependencies(field.Type, owner.file.imports, typeDefs, true, make(map[string]bool)) {
				violations = append(violations, "commandRegistry boundary forbidden type: "+dependency)
			}
		}
		if recordStoreFields != 1 {
			violations = append(violations, fmt.Sprintf("commandRegistry must have one requestRecordAccess field, found %d", recordStoreFields))
		}
	}

	for _, source := range sources {
		for _, decl := range source.file.Decls {
			function, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			ownerFunction := receiverTypeName(function) == "commandRegistry" || function.Name.Name == "newCommandRegistry"
			if ownerFunction {
				for _, fieldList := range []*ast.FieldList{function.Type.Params, function.Type.Results} {
					if fieldList == nil {
						continue
					}
					for _, field := range fieldList.List {
						for _, dependency := range forbiddenTypeDependencies(field.Type, source.imports, typeDefs, true, make(map[string]bool)) {
							violations = append(violations, "commandRegistry boundary forbidden type: "+dependency)
						}
					}
				}
			}
			if source.name == "command_registry.go" {
				ast.Inspect(function.Body, func(node ast.Node) bool {
					call, ok := node.(*ast.CallExpr)
					if !ok {
						return true
					}
					name := calledName(call.Fun)
					switch name {
					case "ReadFile", "WriteFile", "Open", "OpenFile", "Create", "CreateTemp", "readRequestRecord", "writeRequestRecord", "writePrivateAtomic", "writePrivateExclusive":
						violations = append(violations, "command_registry.go bypasses the request record owner with "+name)
					}
					return true
				})
			}
		}
		ast.Inspect(source.file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := calledName(call.Fun)
			if name == "newFileRequestRecordOwner" {
				constructorCalls++
				if !isOneOf(source.name, "endpoint.go", "state.go") {
					violations = append(violations, "file request record owner is constructed outside endpoint composition")
				}
			}
			if name == "readRequestRecord" {
				if source.name != "request_records.go" || !callIsInFileRecordRead(source.file, call) {
					violations = append(violations, "request record reader is defined outside fileRequestRecordOwner.Read")
				}
			}
			if name == "writeRequestRecord" && !strings.HasSuffix(source.name, "_test.go") {
				violations = append(violations, "writeRequestRecord must remain test-only")
			}
			if (source.name == "commands.go" || source.name == "command_registry.go") && isRequestRecordAccessCall(name) {
				if source.name == "command_registry.go" {
					return true // separately reports the more useful owner-bypass violation above
				}
				violations = append(violations, "commands.go bypasses the command registry for request records with "+name)
			}
			return true
		})
	}
	if constructorCalls != 4 { // declaration, HTTP and in-process composition, validation
		violations = append(violations, fmt.Sprintf("newFileRequestRecordOwner must have one declaration and three approved production calls, found %d", constructorCalls))
	}
	sort.Strings(violations)
	return violations
}

func forbiddenTypeDependencies(expression ast.Expr, imports map[string]string, definitions map[string]httpAPITypeDefinition, commandOwner bool, seen map[string]bool) []string {
	var dependencies []string
	ast.Inspect(expression, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.SelectorExpr:
			if packageName, ok := value.X.(*ast.Ident); ok {
				path := imports[packageName.Name]
				switch {
				case path == productClientImport && value.Sel.Name == "Client":
					dependencies = append(dependencies, "product/client.Client")
				case commandOwner && path == "net/http" && isOneOf(value.Sel.Name, "Request", "ResponseWriter", "Client", "Server"):
					dependencies = append(dependencies, "net/http."+value.Sel.Name)
				case path == "database/sql" && isOneOf(value.Sel.Name, "DB", "Conn", "Tx", "Stmt"):
					dependencies = append(dependencies, "database/sql."+value.Sel.Name)
				case path == "net" && value.Sel.Name == "Conn":
					dependencies = append(dependencies, "net.Conn")
				case path == "os" && value.Sel.Name == "File":
					dependencies = append(dependencies, "os.File")
				case path == sqliteImport:
					dependencies = append(dependencies, "storage/sqlite."+value.Sel.Name)
				}
			}
			return false
		case *ast.Ident:
			if commandOwner && value.Name == "Server" {
				dependencies = append(dependencies, "Server")
			}
			if definition, ok := definitions[value.Name]; ok && !seen[value.Name] {
				seen[value.Name] = true
				dependencies = append(dependencies, forbiddenTypeDependencies(definition.expression, definition.imports, definitions, commandOwner, seen)...)
				delete(seen, value.Name)
			}
		}
		return true
	})
	return uniqueStrings(dependencies)
}

func receiverTypeName(function *ast.FuncDecl) string {
	if function.Recv == nil || len(function.Recv.List) == 0 {
		return ""
	}
	expression := function.Recv.List[0].Type
	if pointer, ok := expression.(*ast.StarExpr); ok {
		expression = pointer.X
	}
	if name, ok := expression.(*ast.Ident); ok {
		return name.Name
	}
	return ""
}

func callIsInFileRecordRead(file *ast.File, target *ast.CallExpr) bool {
	for _, decl := range file.Decls {
		function, ok := decl.(*ast.FuncDecl)
		if !ok || function.Name.Name != "Read" || receiverTypeName(function) != "fileRequestRecordOwner" {
			continue
		}
		found := false
		ast.Inspect(function.Body, func(node ast.Node) bool {
			if call, ok := node.(*ast.CallExpr); ok && call == target {
				found = true
			}
			return true
		})
		return found
	}
	return false
}

func hasNamedParameterType(parameters *ast.FieldList, name, typeName string) bool {
	if parameters == nil {
		return false
	}
	for _, field := range parameters.List {
		if !isIdentType(field.Type, typeName) {
			continue
		}
		for _, identifier := range field.Names {
			if identifier.Name == name {
				return true
			}
		}
	}
	return false
}

func isIdentType(expression ast.Expr, name string) bool {
	identifier, ok := expression.(*ast.Ident)
	return ok && identifier.Name == name
}

func fieldName(field *ast.Field) string {
	if len(field.Names) == 0 {
		return ""
	}
	return field.Names[0].Name
}

func calledName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		return value.Sel.Name
	default:
		return ""
	}
}

func isRecordAccessMethod(name string) bool { return isOneOf(name, "Read", "Register", "Write") }

func isRequestRecordAccessInterface(expression ast.Expr) bool {
	interfaceType, ok := expression.(*ast.InterfaceType)
	if !ok || interfaceType.Methods == nil || len(interfaceType.Methods.List) != 3 {
		return false
	}
	found := make(map[string]bool, 3)
	for _, method := range interfaceType.Methods.List {
		if len(method.Names) != 1 || !isRecordAccessMethod(method.Names[0].Name) {
			return false
		}
		found[method.Names[0].Name] = true
	}
	return found["Read"] && found["Register"] && found["Write"]
}

func functionMentionsRequestRecord(function *ast.FuncDecl) bool {
	for _, fieldList := range []*ast.FieldList{function.Type.Params, function.Type.Results} {
		if fieldList == nil {
			continue
		}
		for _, field := range fieldList.List {
			found := false
			ast.Inspect(field.Type, func(node ast.Node) bool {
				if identifier, ok := node.(*ast.Ident); ok && identifier.Name == "requestRecord" {
					found = true
					return false
				}
				return true
			})
			if found {
				return true
			}
		}
	}
	return false
}

func isRequestRecordAccessCall(name string) bool {
	return isOneOf(name, "readRequestRecord", "writeRequestRecord", "writePrivateAtomic", "writePrivateExclusive")
}

func isOneOf(value string, choices ...string) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			unique = append(unique, value)
		}
	}
	return unique
}

func containsViolation(violations []string, wanted string) bool {
	for _, violation := range violations {
		if strings.Contains(violation, wanted) {
			return true
		}
	}
	return false
}
