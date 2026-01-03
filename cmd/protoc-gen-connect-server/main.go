// protoc-gen-connect-server generates Connect RPC service implementations
// Fully generic - works with ANY proto file using proper proto reflection.
// Uses Category Theory: Monoid, Functor (Map), Fold, Filter
//
// Pattern Detection (by TYPE signature, not name):
//   - Get:    Input has ID field referencing entity → Output IS the entity
//   - List:   Output has repeated entity field
//   - Delete: Input has ID field referencing entity → Output is Empty
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
// CATEGORY THEORY FOUNDATIONS
// =============================================================================

type Monoid[A any] struct {
	Empty  func() A
	Append func(A, A) A
}

type Code struct{ Run func() string }

var CodeMonoid = Monoid[Code]{
	Empty:  func() Code { return Code{Run: func() string { return "" }} },
	Append: func(a, b Code) Code { return Code{Run: func() string { return a.Run() + b.Run() }} },
}

func FoldRight[A, B any](xs []A, z B, f func(A, B) B) B {
	if len(xs) == 0 {
		return z
	}
	return f(xs[0], FoldRight(xs[1:], z, f))
}

func Concat[A any](m Monoid[A], xs []A) A {
	return FoldRight(xs, m.Empty(), func(a A, acc A) A { return m.Append(a, acc) })
}

func Map[A, B any](xs []A, f func(A) B) []B {
	return FoldRight(xs, []B{}, func(a A, acc []B) []B { return append([]B{f(a)}, acc...) })
}

func FoldMap[A, B any](xs []A, m Monoid[B], f func(A) B) B {
	return Concat(m, Map(xs, f))
}

func Filter[A any](xs []A, pred func(A) bool) []A {
	return FoldRight(xs, []A{}, func(a A, acc []A) []A {
		if pred(a) {
			return append([]A{a}, acc...)
		}
		return acc
	})
}

// =============================================================================
// CODE PRIMITIVES
// =============================================================================

func Line(s string) Code                    { return Code{Run: func() string { return s + "\n" }} }
func Linef(f string, a ...interface{}) Code { return Line(fmt.Sprintf(f, a...)) }
func Blank() Code                           { return Line("") }
func Comment(s string) Code                 { return Line("// " + s) }

func Indent(c Code) Code {
	return Code{Run: func() string {
		lines := strings.Split(c.Run(), "\n")
		indented := Map(lines, func(l string) string {
			if l == "" {
				return ""
			}
			return "\t" + l
		})
		return strings.Join(indented, "\n")
	}}
}

// =============================================================================
// ENTITY INFO - Extracted via proto reflection
// =============================================================================

type EntityInfo struct {
	GoName    string
	IDField   string
	IDGoName  string
	RepoField string
}

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
	return containsExtension(b, entityExtensionNumber)
}

func containsExtension(b []byte, fieldNum int32) bool {
	tag := uint64(fieldNum<<3 | 2)
	i := 0
	for i < len(b) {
		v, n := decodeVarint(b[i:])
		if n == 0 {
			break
		}
		if v == tag {
			return true
		}
		i += n
		switch v & 0x7 {
		case 0:
			_, vn := decodeVarint(b[i:])
			i += vn
		case 1:
			i += 8
		case 2:
			length, ln := decodeVarint(b[i:])
			i += ln + int(length)
		case 5:
			i += 4
		default:
			return false
		}
	}
	return false
}

func decodeVarint(b []byte) (uint64, int) {
	var x uint64
	for n := 0; n < len(b) && n < 10; n++ {
		x |= uint64(b[n]&0x7f) << (7 * n)
		if b[n] < 0x80 {
			return x, n + 1
		}
	}
	return 0, 0
}

func ExtractEntityInfo(msg *protogen.Message) EntityInfo {
	idField, idGoName := findIDField(msg)
	return EntityInfo{
		GoName:    msg.GoIdent.GoName,
		IDField:   idField,
		IDGoName:  idGoName,
		RepoField: msg.GoIdent.GoName,
	}
}

func findIDField(msg *protogen.Message) (string, string) {
	for _, f := range msg.Fields {
		if strings.EqualFold(string(f.Desc.Name()), "id") {
			return string(f.Desc.Name()), f.GoName
		}
	}
	for _, f := range msg.Fields {
		name := string(f.Desc.Name())
		if strings.HasSuffix(name, "_id") {
			return name, f.GoName
		}
	}
	for _, f := range msg.Fields {
		if f.Desc.Kind() == protoreflect.StringKind {
			return string(f.Desc.Name()), f.GoName
		}
	}
	return "id", "Id"
}

// =============================================================================
// METHOD PATTERN DETECTION - By type signature
// =============================================================================

type MethodPattern int

const (
	PatternUnknown MethodPattern = iota
	PatternGet
	PatternList
	PatternDelete
)

type MethodInfo struct {
	GoName      string
	InputType   string
	OutputType  string
	Pattern     MethodPattern
	Entity      *EntityInfo
	ListField   string
	IDFieldName string
}

func DetectPattern(m *protogen.Method, entities map[string]*EntityInfo) *MethodInfo {
	inputMsg := m.Input
	outputMsg := m.Output
	inputName := inputMsg.GoIdent.GoName
	outputName := outputMsg.GoIdent.GoName

	// Pattern: Output is Empty AND Input has entity ID field → Delete
	if outputName == "Empty" {
		if entity, idField := findEntityIDField(inputMsg, entities); entity != nil {
			return &MethodInfo{
				GoName:      m.GoName,
				InputType:   inputName,
				OutputType:  "emptypb.Empty",
				Pattern:     PatternDelete,
				Entity:      entity,
				IDFieldName: idField,
			}
		}
	}

	// Pattern: Output IS an entity AND Input has that entity's ID field → Get
	if entity, ok := entities[outputName]; ok {
		if idField := findMatchingIDField(inputMsg, entity); idField != "" {
			return &MethodInfo{
				GoName:      m.GoName,
				InputType:   inputName,
				OutputType:  outputName,
				Pattern:     PatternGet,
				Entity:      entity,
				IDFieldName: idField,
			}
		}
	}

	// Pattern: Output has repeated entity field → List
	if entity, listField := findRepeatedEntityField(outputMsg, entities); entity != nil {
		return &MethodInfo{
			GoName:     m.GoName,
			InputType:  fixEmptyType(inputName),
			OutputType: outputName,
			Pattern:    PatternList,
			Entity:     entity,
			ListField:  listField,
		}
	}

	return nil
}

// findEntityIDField finds an ID field in the input message that references any entity
func findEntityIDField(msg *protogen.Message, entities map[string]*EntityInfo) (*EntityInfo, string) {
	for _, f := range msg.Fields {
		if f.Desc.Kind() != protoreflect.StringKind {
			continue
		}
		fieldName := string(f.Desc.Name())

		// Check each entity to see if this field is its ID
		for _, entity := range entities {
			// Match: field name equals entity's ID field
			if fieldName == entity.IDField {
				return entity, f.GoName
			}
			// Match: field name is {lowercase_entity}_id
			expectedID := strings.ToLower(entity.GoName) + "_id"
			if fieldName == expectedID {
				return entity, f.GoName
			}
		}
	}
	return nil, ""
}

// findMatchingIDField finds the ID field for a specific entity
func findMatchingIDField(msg *protogen.Message, entity *EntityInfo) string {
	for _, f := range msg.Fields {
		if f.Desc.Kind() != protoreflect.StringKind {
			continue
		}
		fieldName := string(f.Desc.Name())

		if fieldName == entity.IDField {
			return f.GoName
		}
		expectedID := strings.ToLower(entity.GoName) + "_id"
		if fieldName == expectedID {
			return f.GoName
		}
	}
	return ""
}

// findRepeatedEntityField finds a repeated field containing an entity type
func findRepeatedEntityField(msg *protogen.Message, entities map[string]*EntityInfo) (*EntityInfo, string) {
	for _, f := range msg.Fields {
		if !f.Desc.IsList() || f.Desc.Kind() != protoreflect.MessageKind {
			continue
		}
		elemTypeName := f.Message.GoIdent.GoName
		if entity, ok := entities[elemTypeName]; ok {
			return entity, f.GoName
		}
	}
	return nil, ""
}

func fixEmptyType(t string) string {
	if t == "Empty" {
		return "emptypb.Empty"
	}
	return t
}

// =============================================================================
// CODE GENERATORS
// =============================================================================

func GenGet(svcName string, m *MethodInfo, baseAlias string) Code {
	inputType := baseAlias + "." + m.InputType
	outputType := baseAlias + "." + m.OutputType

	return Concat(CodeMonoid, []Code{
		Blank(),
		Linef("func (s *%sServer) %s(ctx context.Context, req *connect.Request[%s]) (*connect.Response[%s], error) {",
			svcName, m.GoName, inputType, outputType),
		Indent(Concat(CodeMonoid, []Code{
			Linef("id := req.Msg.Get%s()", m.IDFieldName),
			Line(`if id == "" {`),
			Line(`	return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id required"))`),
			Line(`}`),
			Blank(),
			Linef("entity, err := s.repos.%s.Get(ctx, id)", m.Entity.RepoField),
			Line("if err != nil {"),
			Linef("	if errors.Is(err, %s.ErrNotFound) {", baseAlias),
			Line("		return nil, connect.NewError(connect.CodeNotFound, err)"),
			Line("	}"),
			Line("	return nil, connect.NewError(connect.CodeInternal, err)"),
			Line("}"),
			Blank(),
			Line("return connect.NewResponse(entity), nil"),
		})),
		Line("}"),
	})
}

func GenList(svcName string, m *MethodInfo, baseAlias string) Code {
	inputType := m.InputType
	if inputType != "emptypb.Empty" {
		inputType = baseAlias + "." + inputType
	}
	outputType := baseAlias + "." + m.OutputType

	var limitCode Code
	if m.InputType == "emptypb.Empty" {
		limitCode = Line("limit := 100")
	} else {
		limitCode = Concat(CodeMonoid, []Code{
			Line("limit := int(req.Msg.GetLimit())"),
			Line("if limit <= 0 || limit > 100 {"),
			Line("	limit = 100"),
			Line("}"),
		})
	}

	return Concat(CodeMonoid, []Code{
		Blank(),
		Linef("func (s *%sServer) %s(ctx context.Context, req *connect.Request[%s]) (*connect.Response[%s], error) {",
			svcName, m.GoName, inputType, outputType),
		Indent(Concat(CodeMonoid, []Code{
			limitCode,
			Blank(),
			Linef("entities, err := s.repos.%s.List(ctx, limit)", m.Entity.RepoField),
			Line("if err != nil {"),
			Line("	return nil, connect.NewError(connect.CodeInternal, err)"),
			Line("}"),
			Blank(),
			Linef("return connect.NewResponse(&%s.%s{%s: entities}), nil", baseAlias, m.OutputType, m.ListField),
		})),
		Line("}"),
	})
}

func GenDelete(svcName string, m *MethodInfo, baseAlias string) Code {
	inputType := baseAlias + "." + m.InputType

	return Concat(CodeMonoid, []Code{
		Blank(),
		Linef("func (s *%sServer) %s(ctx context.Context, req *connect.Request[%s]) (*connect.Response[emptypb.Empty], error) {",
			svcName, m.GoName, inputType),
		Indent(Concat(CodeMonoid, []Code{
			Linef("id := req.Msg.Get%s()", m.IDFieldName),
			Line(`if id == "" {`),
			Line(`	return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id required"))`),
			Line(`}`),
			Blank(),
			Linef("if err := s.repos.%s.Delete(ctx, id); err != nil {", m.Entity.RepoField),
			Linef("	if errors.Is(err, %s.ErrNotFound) {", baseAlias),
			Line("		return nil, connect.NewError(connect.CodeNotFound, err)"),
			Line("	}"),
			Line("	return nil, connect.NewError(connect.CodeInternal, err)"),
			Line("}"),
			Blank(),
			Line("return connect.NewResponse(&emptypb.Empty{}), nil"),
		})),
		Line("}"),
	})
}

func GenMethod(svcName string, m *MethodInfo, baseAlias string) Code {
	switch m.Pattern {
	case PatternGet:
		return GenGet(svcName, m, baseAlias)
	case PatternList:
		return GenList(svcName, m, baseAlias)
	case PatternDelete:
		return GenDelete(svcName, m, baseAlias)
	default:
		return CodeMonoid.Empty()
	}
}

// =============================================================================
// SERVICE GENERATION
// =============================================================================

type ServiceInfo struct {
	GoName  string
	Methods []*MethodInfo
}

func GenService(svc ServiceInfo, connectAlias string, baseAlias string) Code {
	methods := FoldMap(svc.Methods, CodeMonoid, func(m *MethodInfo) Code {
		return GenMethod(svc.GoName, m, baseAlias)
	})

	return Concat(CodeMonoid, []Code{
		Blank(),
		Comment(fmt.Sprintf("%sServer implements %s", svc.GoName, svc.GoName)),
		Linef("type %sServer struct {", svc.GoName),
		Linef("	%s.Unimplemented%sHandler", connectAlias, svc.GoName),
		Linef("	repos *%s.Repositories", baseAlias),
		Line("}"),
		Blank(),
		Linef("func New%sServer(repos *%s.Repositories) *%sServer {", svc.GoName, baseAlias, svc.GoName),
		Linef("	return &%sServer{repos: repos}", svc.GoName),
		Line("}"),
		methods,
	})
}

func GenServiceSet(services []ServiceInfo) Code {
	if len(services) == 0 {
		return CodeMonoid.Empty()
	}

	providers := FoldMap(services, CodeMonoid, func(svc ServiceInfo) Code {
		return Linef("	New%sServer,", svc.GoName)
	})

	return Concat(CodeMonoid, []Code{
		Blank(),
		Comment("ServiceServerSet provides all generated service servers for Wire."),
		Line("var ServiceServerSet = wire.NewSet("),
		providers,
		Line(")"),
	})
}

func GenFile(pkgName string, services []ServiceInfo, connectPkg string, basePkg string) Code {
	// Extract package alias from connect path
	connectParts := strings.Split(connectPkg, "/")
	connectAlias := connectParts[len(connectParts)-1]

	// Use fixed alias for base package
	baseAlias := "pb"

	svcCode := FoldMap(services, CodeMonoid, func(svc ServiceInfo) Code {
		return GenService(svc, connectAlias, baseAlias)
	})

	return Concat(CodeMonoid, []Code{
		Comment("Code generated by protoc-gen-connect-server. DO NOT EDIT."),
		Comment("Pattern-based generation using proto reflection."),
		Blank(),
		Linef("package %s", pkgName),
		Blank(),
		Line("import ("),
		Line(`	"context"`),
		Line(`	"errors"`),
		Blank(),
		Line(`	"connectrpc.com/connect"`),
		Line(`	"github.com/google/wire"`),
		Line(`	"google.golang.org/protobuf/types/known/emptypb"`),
		Linef(`	%s "%s"`, baseAlias, basePkg),
		Linef(`	"%s"`, connectPkg),
		Line(")"),
		Blank(),
		Comment("Ensure imports"),
		Line("var ("),
		Line("	_ = wire.NewSet"),
		Line("	_ = emptypb.Empty{}"),
		Line("	_ = errors.New"),
		Line("	_ = connect.CodeOK"),
		Line(")"),
		svcCode,
		GenServiceSet(services),
	})
}

// =============================================================================
// MAIN - Pure functional pipeline
// =============================================================================

func main() {
	protogen.Options{}.Run(func(gen *protogen.Plugin) error {
		gen.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)

		for _, f := range gen.Files {
			if !f.Generate {
				continue
			}

			// Step 1: Extract entities (messages with entity option)
			entities := make(map[string]*EntityInfo)
			for _, msg := range f.Messages {
				if hasEntityOption(msg) {
					info := ExtractEntityInfo(msg)
					entities[info.GoName] = &info
				}
			}

			if len(entities) == 0 {
				continue
			}

			// Step 2: Analyze services - detect patterns by type signature
			var services []ServiceInfo
			for _, svc := range f.Services {
				methods := Filter(
					Map(svc.Methods, func(m *protogen.Method) *MethodInfo {
						return DetectPattern(m, entities)
					}),
					func(m *MethodInfo) bool { return m != nil },
				)

				if len(methods) > 0 {
					services = append(services, ServiceInfo{
						GoName:  svc.GoName,
						Methods: methods,
					})
				}
			}

			if len(services) == 0 {
				continue
			}

			// Compute packages
			basePkg := string(f.GoImportPath)
			connectPkg := basePkg + "/" + strings.ToLower(string(f.GoPackageName)) + "connect"
			serversPkgName := "servers"
			serversPkgPath := basePkg + "/" + serversPkgName

			// Step 3: Generate code into servers subpackage
			// f.GeneratedFilenamePrefix is like "path/to/pkg" - we want "path/to/servers/servers.pb.go"
			parts := strings.Split(f.GeneratedFilenamePrefix, "/")
			if len(parts) > 0 {
				parts[len(parts)-1] = "servers"
			}
			outputPath := strings.Join(parts, "/") + "/servers.pb.go"

			g := gen.NewGeneratedFile(outputPath, protogen.GoImportPath(serversPkgPath))
			g.P(GenFile(serversPkgName, services, connectPkg, basePkg).Run())
		}
		return nil
	})
}
