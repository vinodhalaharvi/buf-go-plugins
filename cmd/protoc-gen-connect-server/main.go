// protoc-gen-connect-server generates Connect RPC service implementations
// wired to Firestore repositories. Only generates Get/List/Delete.
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
// MONOID + HELPERS
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
func comment(s string) Code                 { return line("// " + s) }

func indent(c Code) Code {
	return Code{Run: func() string {
		var result []string
		for _, l := range strings.Split(c.Run(), "\n") {
			if l == "" {
				result = append(result, "")
			} else {
				result = append(result, "\t"+l)
			}
		}
		return strings.Join(result, "\n")
	}}
}

// =============================================================================
// ENTITY CONFIG
// =============================================================================

type EntityConfig struct {
	GoName   string
	IDGoName string
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

func getEntityConfig(msg *protogen.Message) *EntityConfig {
	if !hasEntityOption(msg) {
		return nil
	}
	idGoName := "Id"
	for _, f := range msg.Fields {
		name := string(f.Desc.Name())
		if strings.EqualFold(name, "id") {
			idGoName = f.GoName
			break
		}
		if strings.HasSuffix(strings.ToLower(name), "_id") && idGoName == "Id" {
			idGoName = f.GoName
		}
	}
	return &EntityConfig{GoName: msg.GoIdent.GoName, IDGoName: idGoName}
}

// =============================================================================
// METHOD INFO
// =============================================================================

type MethodInfo struct {
	GoName     string
	InputType  string
	OutputType string
	OutputMsg  *protogen.Message
	Entity     string
	Config     *EntityConfig
	Op         string
}

func extractMethods(svc *protogen.Service, entities map[string]*EntityConfig, file *protogen.File) []MethodInfo {
	var methods []MethodInfo
	for _, m := range svc.Methods {
		name := string(m.Desc.Name())
		op := ""
		entity := ""

		if strings.HasPrefix(name, "Get") {
			op = "get"
			entity = strings.TrimPrefix(name, "Get")
		} else if strings.HasPrefix(name, "List") {
			op = "list"
			entity = strings.TrimSuffix(strings.TrimPrefix(name, "List"), "s")
		} else if strings.HasPrefix(name, "Delete") {
			op = "delete"
			entity = strings.TrimPrefix(name, "Delete")
		}

		if op == "" {
			continue
		}

		// Skip special methods
		if entity == "CurrentUser" || entity == "CurrentSession" || entity == "Subscription" ||
			entity == "BillingPortalUrl" || entity == "Usage" || entity == "TenantStat" ||
			entity == "2FAStatus" || entity == "CAChain" || entity == "CRL" {
			continue
		}

		config := entities[entity]
		if config == nil {
			continue
		}

		// Find output message
		var outputMsg *protogen.Message
		for _, msg := range file.Messages {
			if msg.GoIdent.GoName == m.Output.GoIdent.GoName {
				outputMsg = msg
				break
			}
		}

		methods = append(methods, MethodInfo{
			GoName:     m.GoName,
			InputType:  fixEmptyType(m.Input.GoIdent.GoName),
			OutputType: fixEmptyType(m.Output.GoIdent.GoName),
			OutputMsg:  outputMsg,
			Entity:     entity,
			Config:     config,
			Op:         op,
		})
	}
	return methods
}

// =============================================================================
// CODE GENERATORS
// =============================================================================

func genGet(m MethodInfo) Code {
	e := lowerFirst(m.Entity)
	return concat(
		linef("id := req.Msg.Get%s()", m.Config.IDGoName),
		line(`if id == "" {`),
		line(`	return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))`),
		line(`}`),
		blank(),
		linef("entity, err := s.%sRepo.Get(ctx, id)", e),
		line("if err != nil {"),
		line("	if errors.Is(err, ErrNotFound) {"),
		line("		return nil, connect.NewError(connect.CodeNotFound, err)"),
		line("	}"),
		line("	return nil, connect.NewError(connect.CodeInternal, err)"),
		line("}"),
		blank(),
		line("return connect.NewResponse(entity), nil"),
	)
}

func genList(m MethodInfo) Code {
	e := lowerFirst(m.Entity)

	// Find the repeated field name in output message
	listField := m.Entity + "s"
	if m.OutputMsg != nil {
		for _, f := range m.OutputMsg.Fields {
			if f.Desc.IsList() && f.Desc.Kind() == protoreflect.MessageKind {
				listField = f.GoName
				break
			}
		}
	}

	return concat(
		line("limit := int(req.Msg.GetLimit())"),
		line("if limit <= 0 || limit > 100 {"),
		line("	limit = 100"),
		line("}"),
		blank(),
		linef("entities, err := s.%sRepo.List(ctx, limit)", e),
		line("if err != nil {"),
		line("	return nil, connect.NewError(connect.CodeInternal, err)"),
		line("}"),
		blank(),
		linef("return connect.NewResponse(&%s{%s: entities}), nil", m.OutputType, listField),
	)
}

func genDelete(m MethodInfo) Code {
	e := lowerFirst(m.Entity)
	return concat(
		linef("id := req.Msg.Get%s()", m.Config.IDGoName),
		line(`if id == "" {`),
		line(`	return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))`),
		line(`}`),
		blank(),
		linef("if err := s.%sRepo.Delete(ctx, id); err != nil {", e),
		line("	if errors.Is(err, ErrNotFound) {"),
		line("		return nil, connect.NewError(connect.CodeNotFound, err)"),
		line("	}"),
		line("	return nil, connect.NewError(connect.CodeInternal, err)"),
		line("}"),
		blank(),
		line("return connect.NewResponse(&emptypb.Empty{}), nil"),
	)
}

func genMethod(svcName string, m MethodInfo) Code {
	recv := fmt.Sprintf("s *%sServer", svcName)
	params := fmt.Sprintf("ctx context.Context, req *connect.Request[%s]", m.InputType)
	returns := fmt.Sprintf("(*connect.Response[%s], error)", m.OutputType)

	var body Code
	switch m.Op {
	case "get":
		body = genGet(m)
	case "list":
		body = genList(m)
	case "delete":
		body = genDelete(m)
	}

	return concat(
		blank(),
		linef("func (%s) %s(%s) %s {", recv, m.GoName, params, returns),
		indent(body),
		line("}"),
	)
}

func genService(svcName string, methods []MethodInfo) Code {
	// Collect entities
	entities := make(map[string]bool)
	for _, m := range methods {
		entities[m.Entity] = true
	}

	// Struct fields
	fields := empty
	for e := range entities {
		fields = append2(fields, linef("%sRepo *Firestore%sRepository", lowerFirst(e), e))
	}

	// Constructor params and assigns
	var params []string
	assigns := empty
	for e := range entities {
		params = append(params, fmt.Sprintf("%sRepo *Firestore%sRepository", lowerFirst(e), e))
		assigns = append2(assigns, linef("	%sRepo: %sRepo,", lowerFirst(e), lowerFirst(e)))
	}

	// Methods
	methodsCode := empty
	for _, m := range methods {
		methodsCode = append2(methodsCode, genMethod(svcName, m))
	}

	return concat(
		blank(),
		linef("type %sServer struct {", svcName),
		indent(fields),
		line("}"),
		blank(),
		linef("func New%sServer(%s) *%sServer {", svcName, strings.Join(params, ", "), svcName),
		linef("	return &%sServer{", svcName),
		assigns,
		line("	}"),
		line("}"),
		methodsCode,
	)
}

func genFile(file *protogen.File, services map[string][]MethodInfo) Code {
	svcCode := empty
	for svcName, methods := range services {
		svcCode = append2(svcCode, genService(svcName, methods))
	}

	return concat(
		comment("Code generated by protoc-gen-connect-server. DO NOT EDIT."),
		blank(),
		linef("package %s", file.GoPackageName),
		blank(),
		line("import ("),
		line(`	"context"`),
		line(`	"errors"`),
		blank(),
		line(`	"connectrpc.com/connect"`),
		line(`	"google.golang.org/protobuf/types/known/emptypb"`),
		line(")"),
		svcCode,
	)
}

func main() {
	protogen.Options{}.Run(func(gen *protogen.Plugin) error {
		gen.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)

		for _, f := range gen.Files {
			if !f.Generate {
				continue
			}

			// Get entities
			entities := make(map[string]*EntityConfig)
			for _, msg := range f.Messages {
				if cfg := getEntityConfig(msg); cfg != nil {
					entities[msg.GoIdent.GoName] = cfg
				}
			}
			if len(entities) == 0 {
				continue
			}

			// Get service methods
			services := make(map[string][]MethodInfo)
			for _, svc := range f.Services {
				methods := extractMethods(svc, entities, f)
				if len(methods) > 0 {
					services[svc.GoName] = methods
				}
			}
			if len(services) == 0 {
				continue
			}

			g := gen.NewGeneratedFile(f.GeneratedFilenamePrefix+"_connect_server.pb.go", f.GoImportPath)
			g.P(genFile(f, services).Run())
		}
		return nil
	})
}

func lowerFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func fixEmptyType(t string) string {
	if t == "Empty" {
		return "emptypb.Empty"
	}
	return t
}
