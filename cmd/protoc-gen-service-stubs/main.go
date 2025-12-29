// protoc-gen-service-stubs generates Connect service implementations
// that wire to Firestore repositories. CRUD methods are implemented,
// complex methods return Unimplemented (override as needed).
package main

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	pluginpb "google.golang.org/protobuf/types/pluginpb"
)

const entityExtensionNumber = 50000

// =============================================================================
// CODE HELPERS
// =============================================================================

type Code struct{ Run func() string }

var empty = Code{Run: func() string { return "" }}

func append2(a, b Code) Code { return Code{Run: func() string { return a.Run() + b.Run() }} }

func concat(codes ...Code) Code {
	result := empty
	for _, c := range codes {
		result = append2(result, c)
	}
	return result
}

func line(s string) Code                    { return Code{Run: func() string { return s + "\n" }} }
func linef(f string, a ...interface{}) Code { return line(fmt.Sprintf(f, a...)) }
func blank() Code                           { return line("") }

// =============================================================================
// ENTITY DETECTION
// =============================================================================

func hasEntityOption(msg *protogen.Message) bool {
	opts := msg.Desc.Options()
	if opts == nil {
		return false
	}
	optsProto, ok := opts.(*descriptorpb.MessageOptions)
	if !ok {
		return false
	}
	b, _ := proto.Marshal(optsProto)
	return containsTag(b, entityExtensionNumber)
}

func containsTag(b []byte, fieldNum int32) bool {
	tag := uint64(fieldNum<<3 | 2)
	i := 0
	for i < len(b) {
		v, n := varint(b[i:])
		if n == 0 {
			break
		}
		if v == tag {
			return true
		}
		i += n
		switch v & 0x7 {
		case 0:
			_, vn := varint(b[i:])
			i += vn
		case 1:
			i += 8
		case 2:
			length, ln := varint(b[i:])
			i += ln + int(length)
		case 5:
			i += 4
		default:
			return false
		}
	}
	return false
}

func varint(b []byte) (uint64, int) {
	var x uint64
	for n := 0; n < len(b) && n < 10; n++ {
		x |= uint64(b[n]&0x7f) << (7 * n)
		if b[n] < 0x80 {
			return x, n + 1
		}
	}
	return 0, 0
}

// =============================================================================
// INFO TYPES
// =============================================================================

type EntityInfo struct {
	GoName  string
	IDField string
}

type MethodInfo struct {
	GoName     string
	InputType  string
	OutputType string
	Op         string // "get", "list", "delete", "create", "update", or ""
	Entity     string
	IDField    string
	ListField  string // for list responses
	OutputMsg  *protogen.Message
}

type ServiceInfo struct {
	GoName  string
	Methods []MethodInfo
}

// =============================================================================
// ANALYSIS
// =============================================================================

func analyzeMethod(m *protogen.Method, entities map[string]*EntityInfo, file *protogen.File) MethodInfo {
	name := string(m.Desc.Name())
	inputType := m.Input.GoIdent.GoName
	outputType := m.Output.GoIdent.GoName

	// Fix Empty types
	if inputType == "Empty" {
		inputType = "emptypb.Empty"
	}
	if outputType == "Empty" {
		outputType = "emptypb.Empty"
	}

	info := MethodInfo{
		GoName:     m.GoName,
		InputType:  inputType,
		OutputType: outputType,
	}

	// Detect operation and entity
	var op, entity string
	if strings.HasPrefix(name, "Get") {
		op = "get"
		entity = strings.TrimPrefix(name, "Get")
	} else if strings.HasPrefix(name, "List") {
		op = "list"
		entity = strings.TrimSuffix(strings.TrimPrefix(name, "List"), "s")
	} else if strings.HasPrefix(name, "Delete") {
		op = "delete"
		entity = strings.TrimPrefix(name, "Delete")
	} else if strings.HasPrefix(name, "Create") {
		op = "create"
		entity = strings.TrimPrefix(name, "Create")
	} else if strings.HasPrefix(name, "Update") {
		op = "update"
		entity = strings.TrimPrefix(name, "Update")
	}

	// Check if entity exists
	if entityInfo, ok := entities[entity]; ok {
		info.Op = op
		info.Entity = entity
		info.IDField = entityInfo.IDField

		// Find list field for list operations
		if op == "list" {
			for _, msg := range file.Messages {
				if msg.GoIdent.GoName == m.Output.GoIdent.GoName {
					info.OutputMsg = msg
					for _, f := range msg.Fields {
						if f.Desc.IsList() && f.Desc.Kind() == protoreflect.MessageKind {
							info.ListField = f.GoName
							break
						}
					}
					break
				}
			}
		}
	}

	return info
}

func getIDField(msg *protogen.Message) string {
	for _, f := range msg.Fields {
		name := string(f.Desc.Name())
		if strings.EqualFold(name, "id") {
			return f.GoName
		}
		if strings.HasSuffix(strings.ToLower(name), "_id") {
			return f.GoName
		}
	}
	return "Id"
}

// =============================================================================
// CODE GENERATION
// =============================================================================

func generateFile(file *protogen.File, services []ServiceInfo, entities map[string]*EntityInfo, pkgName, connectPkg string) Code {
	return concat(
		generateHeader(pkgName, connectPkg),
		generateServices(services, entities, connectPkg),
		generateWireProviders(services),
	)
}

func generateHeader(pkgName, connectPkg string) Code {
	return concat(
		line("// Code generated by protoc-gen-service-stubs. DO NOT EDIT."),
		line("// Service implementations wired to Firestore repositories."),
		line("// Override methods as needed for custom business logic."),
		blank(),
		linef("package %s", pkgName),
		blank(),
		line("import ("),
		line(`	"context"`),
		line(`	"errors"`),
		blank(),
		line(`	"connectrpc.com/connect"`),
		line(`	"github.com/google/wire"`),
		line(`	"google.golang.org/protobuf/types/known/emptypb"`),
		linef(`	"%s"`, connectPkg),
		line(")"),
		blank(),
		line("// Ensure imports are used"),
		line("var ("),
		line("	_ = emptypb.Empty{}"),
		line("	_ = wire.NewSet"),
		line(")"),
		blank(),
	)
}

func generateServices(services []ServiceInfo, entities map[string]*EntityInfo, connectPkg string) Code {
	result := empty
	for _, svc := range services {
		result = append2(result, generateService(svc, entities, connectPkg))
	}
	return result
}

func generateService(svc ServiceInfo, entities map[string]*EntityInfo, connectPkg string) Code {
	// All services use the shared Repositories struct
	connectPkgName := extractPkgName(connectPkg)

	// Build methods
	methods := empty
	for _, m := range svc.Methods {
		methods = append2(methods, generateMethod(svc.GoName, m, connectPkg))
	}

	return concat(
		line("// ============================================================================="),
		linef("// %s", strings.ToUpper(svc.GoName)),
		line("// ============================================================================="),
		blank(),
		linef("type %s struct {", svc.GoName),
		linef("	%s.Unimplemented%sHandler", connectPkgName, svc.GoName),
		line("	repos *Repositories"),
		line("}"),
		blank(),
		linef("func New%s(repos *Repositories) *%s {", svc.GoName, svc.GoName),
		linef("	return &%s{repos: repos}", svc.GoName),
		line("}"),
		methods,
		blank(),
	)
}

func generateMethod(svcName string, m MethodInfo, connectPkg string) Code {
	recv := fmt.Sprintf("s *%s", svcName)
	params := fmt.Sprintf("ctx context.Context, req *connect.Request[%s]", m.InputType)
	returns := fmt.Sprintf("(*connect.Response[%s], error)", m.OutputType)

	var body Code
	switch m.Op {
	case "get":
		body = generateGet(m)
	case "list":
		body = generateList(m)
	case "delete":
		body = generateDelete(m)
	case "create":
		body = generateCreate(m)
	case "update":
		body = generateUpdate(m)
	default:
		body = generateUnimplemented()
	}

	return concat(
		blank(),
		linef("func (%s) %s(%s) %s {", recv, m.GoName, params, returns),
		body,
		line("}"),
	)
}

func generateGet(m MethodInfo) Code {
	e := m.Entity
	return concat(
		linef("	id := req.Msg.Get%s()", m.IDField),
		line(`	if id == "" {`),
		line(`		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id required"))`),
		line(`	}`),
		linef("	entity, err := s.repos.%s.Get(ctx, id)", e),
		line("	if err != nil {"),
		line("		if errors.Is(err, ErrNotFound) {"),
		line("			return nil, connect.NewError(connect.CodeNotFound, err)"),
		line("		}"),
		line("		return nil, connect.NewError(connect.CodeInternal, err)"),
		line("	}"),
		line("	return connect.NewResponse(entity), nil"),
	)
}

func generateList(m MethodInfo) Code {
	e := m.Entity
	listField := m.ListField
	if listField == "" {
		listField = m.Entity + "s"
	}

	// Handle Empty input (no pagination)
	if m.InputType == "emptypb.Empty" {
		return concat(
			linef("	entities, err := s.repos.%s.List(ctx, 100)", e),
			line("	if err != nil {"),
			line("		return nil, connect.NewError(connect.CodeInternal, err)"),
			line("	}"),
			linef("	return connect.NewResponse(&%s{%s: entities}), nil", m.OutputType, listField),
		)
	}

	return concat(
		line("	limit := int(req.Msg.GetLimit())"),
		line("	if limit <= 0 || limit > 100 {"),
		line("		limit = 100"),
		line("	}"),
		linef("	entities, err := s.repos.%s.List(ctx, limit)", e),
		line("	if err != nil {"),
		line("		return nil, connect.NewError(connect.CodeInternal, err)"),
		line("	}"),
		linef("	return connect.NewResponse(&%s{%s: entities}), nil", m.OutputType, listField),
	)
}

func generateDelete(m MethodInfo) Code {
	e := m.Entity
	return concat(
		linef("	id := req.Msg.Get%s()", m.IDField),
		line(`	if id == "" {`),
		line(`		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id required"))`),
		line(`	}`),
		linef("	if err := s.repos.%s.Delete(ctx, id); err != nil {", e),
		line("		if errors.Is(err, ErrNotFound) {"),
		line("			return nil, connect.NewError(connect.CodeNotFound, err)"),
		line("		}"),
		line("		return nil, connect.NewError(connect.CodeInternal, err)"),
		line("	}"),
		line("	return connect.NewResponse(&emptypb.Empty{}), nil"),
	)
}

func generateCreate(m MethodInfo) Code {
	// Create operations need custom logic (validation, password hashing, etc.)
	// Generate a TODO stub
	return concat(
		line("	// TODO: Implement create logic"),
		line("	// - Validate input"),
		line("	// - Set defaults (CreatedAt, UpdatedAt)"),
		line("	// - Call s.xxxRepo.Create(ctx, entity)"),
		line(`	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not implemented"))`),
	)
}

func generateUpdate(m MethodInfo) Code {
	// Update operations need custom logic
	return concat(
		line("	// TODO: Implement update logic"),
		line("	// - Get existing entity"),
		line("	// - Apply updates from request"),
		line("	// - Set UpdatedAt"),
		line("	// - Call s.xxxRepo.Update(ctx, entity)"),
		line(`	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not implemented"))`),
	)
}

func generateUnimplemented() Code {
	return line(`	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not implemented"))`)
}

func generateWireProviders(services []ServiceInfo) Code {
	providers := empty
	for _, svc := range services {
		providers = append2(providers, linef("	New%s,", svc.GoName))
	}

	return concat(
		line("// ============================================================================="),
		line("// WIRE PROVIDERS"),
		line("// ============================================================================="),
		blank(),
		line("// ServiceSet provides all service constructors for Wire."),
		line("var ServiceSet = wire.NewSet("),
		providers,
		line(")"),
		blank(),
	)
}

// =============================================================================
// MAIN
// =============================================================================

func main() {
	protogen.Options{}.Run(func(gen *protogen.Plugin) error {
		gen.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)

		for _, f := range gen.Files {
			if !f.Generate || len(f.Services) == 0 {
				continue
			}

			// Build entity map
			entities := make(map[string]*EntityInfo)
			for _, msg := range f.Messages {
				if hasEntityOption(msg) {
					entities[msg.GoIdent.GoName] = &EntityInfo{
						GoName:  msg.GoIdent.GoName,
						IDField: getIDField(msg),
					}
				}
			}

			// Build service info
			var services []ServiceInfo
			for _, svc := range f.Services {
				svcInfo := ServiceInfo{GoName: svc.GoName}
				for _, m := range svc.Methods {
					svcInfo.Methods = append(svcInfo.Methods, analyzeMethod(m, entities, f))
				}
				services = append(services, svcInfo)
			}

			pkgName := string(f.GoPackageName)
			// Construct connect package path
			connectPkg := string(f.GoImportPath) + "/" + strings.ToLower(pkgName) + "connect"

			g := gen.NewGeneratedFile(f.GeneratedFilenamePrefix+"_services.pb.go", f.GoImportPath)
			g.P(generateFile(f, services, entities, pkgName, connectPkg).Run())
		}
		return nil
	})
}

// =============================================================================
// HELPERS
// =============================================================================

func lowerFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func extractPkgName(importPath string) string {
	parts := strings.Split(importPath, "/")
	return parts[len(parts)-1]
}
