// protoc-gen-connect-server generates Connect RPC service implementations
// wired to repositories, plus HTTP server setup.
// Uses Category Theory: Monoid + Functor + Fold
package main

import (
	"fmt"
	"strings"
	"unicode"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoreflect"
	pluginpb "google.golang.org/protobuf/types/pluginpb"
)

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

func FoldMap[A, B any](xs []A, m Monoid[B], f func(A) B) B { return Concat(m, Map(xs, f)) }

func Filter[A any](xs []A, pred func(A) bool) []A {
	return FoldRight(xs, []A{}, func(a A, acc []A) []A {
		if pred(a) {
			return append([]A{a}, acc...)
		}
		return acc
	})
}

func When(cond bool, c Code) Code {
	if cond {
		return c
	}
	return CodeMonoid.Empty()
}

// =============================================================================
// CODE PRIMITIVES
// =============================================================================

func Line(s string) Code                               { return Code{Run: func() string { return s + "\n" }} }
func Linef(format string, args ...interface{}) Code    { return Line(fmt.Sprintf(format, args...)) }
func Blank() Code                                      { return Line("") }
func Comment(text string) Code                         { return Line("// " + text) }
func Commentf(format string, args ...interface{}) Code { return Comment(fmt.Sprintf(format, args...)) }

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

func Package(name string) Code { return Line("package " + name) }

func Import(path string) Code {
	if path == "" {
		return Line("")
	}
	return Linef("\t%q", path)
}

func Imports(paths ...string) Code {
	return Concat(CodeMonoid, []Code{Blank(), Line("import ("), FoldMap(paths, CodeMonoid, Import), Line(")")})
}

func Struct(name string, fields Code) Code {
	return Concat(CodeMonoid, []Code{Linef("type %s struct {", name), Indent(fields), Line("}")})
}

func Field(name, typ string) Code { return Linef("%s %s", name, typ) }

func Func(name, params, returns string, body Code) Code {
	sig := fmt.Sprintf("func %s(%s)", name, params)
	if returns != "" {
		sig += " " + returns
	}
	return Concat(CodeMonoid, []Code{Line(sig + " {"), Indent(body), Line("}")})
}

func Method(receiver, name, params, returns string, body Code) Code {
	sig := fmt.Sprintf("func (%s) %s(%s)", receiver, name, params)
	if returns != "" {
		sig += " " + returns
	}
	return Concat(CodeMonoid, []Code{Line(sig + " {"), Indent(body), Line("}")})
}

func If(cond string, body Code) Code {
	return Concat(CodeMonoid, []Code{Linef("if %s {", cond), Indent(body), Line("}")})
}

func Return(values ...string) Code {
	if len(values) == 0 {
		return Line("return")
	}
	return Linef("return %s", strings.Join(values, ", "))
}

// =============================================================================
// SERVICE/MESSAGE INFO
// =============================================================================

type ServiceInfo struct {
	Name, GoName string
	Methods      []MethodInfo
}

type MethodInfo struct {
	Name, GoName       string
	InputType, OutType string
	IsStreaming        bool
	EntityName         string // Derived from method name
	Operation          string // create, get, list, update, delete
}

type MessageInfo struct {
	Name, GoName                                    string
	HasID, HasCreatedAt, HasUpdatedAt, HasDeletedAt bool
	Fields                                          []FieldInfo
}

type FieldInfo struct {
	Name, GoName, GoType      string
	IsID, IsIndexed, IsUnique bool
}

func ExtractServiceInfo(svc *protogen.Service) ServiceInfo {
	methods := Map(svc.Methods, func(m *protogen.Method) MethodInfo {
		name := string(m.Desc.Name())
		return MethodInfo{
			Name:        name,
			GoName:      m.GoName,
			InputType:   m.Input.GoIdent.GoName,
			OutType:     m.Output.GoIdent.GoName,
			IsStreaming: m.Desc.IsStreamingServer() || m.Desc.IsStreamingClient(),
			EntityName:  extractEntityName(name),
			Operation:   extractOperation(name),
		}
	})
	return ServiceInfo{
		Name:    string(svc.Desc.Name()),
		GoName:  svc.GoName,
		Methods: methods,
	}
}

func ExtractMessageInfo(msg *protogen.Message) MessageInfo {
	fields := Map(msg.Fields, func(f *protogen.Field) FieldInfo {
		name := string(f.Desc.Name())
		isUnique := strings.EqualFold(name, "email") || strings.EqualFold(name, "slug")
		return FieldInfo{
			Name:      name,
			GoName:    f.GoName,
			GoType:    fieldGoType(f),
			IsID:      strings.EqualFold(name, "id"),
			IsUnique:  isUnique,
			IsIndexed: isUnique || strings.HasSuffix(strings.ToLower(name), "_id"),
		}
	})

	hasID, hasCreatedAt, hasUpdatedAt, hasDeletedAt := false, false, false, false
	for _, f := range fields {
		snake := toSnakeCase(f.Name)
		switch {
		case f.IsID:
			hasID = true
		case snake == "created_at":
			hasCreatedAt = true
		case snake == "updated_at":
			hasUpdatedAt = true
		case snake == "deleted_at":
			hasDeletedAt = true
		}
	}

	return MessageInfo{
		Name:         string(msg.Desc.Name()),
		GoName:       msg.GoIdent.GoName,
		HasID:        hasID,
		HasCreatedAt: hasCreatedAt,
		HasUpdatedAt: hasUpdatedAt,
		HasDeletedAt: hasDeletedAt,
		Fields:       fields,
	}
}

func extractEntityName(methodName string) string {
	// CreateUser -> User, GetUser -> User, ListUsers -> User
	for _, prefix := range []string{"Create", "Get", "List", "Update", "Delete", "SoftDelete", "Restore"} {
		if strings.HasPrefix(methodName, prefix) {
			entity := strings.TrimPrefix(methodName, prefix)
			entity = strings.TrimSuffix(entity, "s") // ListUsers -> User
			return entity
		}
	}
	return methodName
}

func extractOperation(methodName string) string {
	lower := strings.ToLower(methodName)
	switch {
	case strings.HasPrefix(lower, "create"):
		return "create"
	case strings.HasPrefix(lower, "get"):
		return "get"
	case strings.HasPrefix(lower, "list"):
		return "list"
	case strings.HasPrefix(lower, "update"):
		return "update"
	case strings.HasPrefix(lower, "delete"):
		return "delete"
	case strings.HasPrefix(lower, "softdelete"):
		return "softdelete"
	case strings.HasPrefix(lower, "restore"):
		return "restore"
	default:
		return "custom"
	}
}

func fieldGoType(field *protogen.Field) string {
	switch field.Desc.Kind() {
	case protoreflect.StringKind:
		return "string"
	case protoreflect.Int32Kind:
		return "int32"
	case protoreflect.Int64Kind:
		return "int64"
	case protoreflect.BoolKind:
		return "bool"
	case protoreflect.EnumKind:
		return field.Enum.GoIdent.GoName
	case protoreflect.MessageKind:
		if field.Message.GoIdent.GoName == "Timestamp" {
			return "*timestamppb.Timestamp"
		}
		return "*" + field.Message.GoIdent.GoName
	default:
		return "interface{}"
	}
}

// =============================================================================
// CONNECT SERVICE GENERATORS
// =============================================================================

func Header() Code {
	return Concat(CodeMonoid, []Code{
		Comment("Code generated by protoc-gen-connect-server. DO NOT EDIT."),
		Comment("Generated using Category Theory: Monoid + Functor + Fold"),
		Comment("Connect RPC service implementations with repository wiring."),
	})
}

func ServiceStruct(s ServiceInfo, messages []MessageInfo) Code {
	// Collect unique entity names to create repository fields
	entities := make(map[string]bool)
	for _, m := range s.Methods {
		if m.EntityName != "" {
			entities[m.EntityName] = true
		}
	}

	repoFields := Concat(CodeMonoid, []Code{})
	for entity := range entities {
		repoFields = CodeMonoid.Append(repoFields, Linef("%sRepo %sRepository", lowerFirst(entity), entity))
	}

	return Concat(CodeMonoid, []Code{
		Blank(),
		Commentf("%sServer implements the %s Connect service", s.GoName, s.GoName),
		Struct(s.GoName+"Server", repoFields),
	})
}

func ServiceConstructor(s ServiceInfo) Code {
	entities := make(map[string]bool)
	for _, m := range s.Methods {
		if m.EntityName != "" {
			entities[m.EntityName] = true
		}
	}

	params := []string{}
	assigns := []Code{}
	for entity := range entities {
		params = append(params, fmt.Sprintf("%sRepo %sRepository", lowerFirst(entity), entity))
		assigns = append(assigns, Linef("%sRepo: %sRepo,", lowerFirst(entity), lowerFirst(entity)))
	}

	return Concat(CodeMonoid, []Code{
		Blank(),
		Commentf("New%sServer creates a new Connect service server", s.GoName),
		Func("New"+s.GoName+"Server", strings.Join(params, ", "), "*"+s.GoName+"Server",
			Concat(CodeMonoid, append([]Code{Linef("return &%sServer{", s.GoName)}, append(assigns, Line("}"))...))),
	})
}

func ServiceMethod(s ServiceInfo, m MethodInfo) Code {
	recv := "s *" + s.GoName + "Server"
	params := fmt.Sprintf("ctx context.Context, req *connect.Request[%s]", m.InputType)
	returns := fmt.Sprintf("(*connect.Response[%s], error)", m.OutType)

	var body Code
	switch m.Operation {
	case "create":
		body = createMethodBody(m)
	case "get":
		body = getMethodBody(m)
	case "list":
		body = listMethodBody(m)
	case "update":
		body = updateMethodBody(m)
	case "delete":
		body = deleteMethodBody(m)
	case "softdelete":
		body = softDeleteMethodBody(m)
	case "restore":
		body = restoreMethodBody(m)
	default:
		body = customMethodBody(m)
	}

	return Concat(CodeMonoid, []Code{
		Blank(),
		Commentf("%s implements %s.%s", m.GoName, s.GoName, m.GoName),
		Method(recv, m.GoName, params, returns, body),
	})
}

func createMethodBody(m MethodInfo) Code {
	entity := lowerFirst(m.EntityName)
	return Concat(CodeMonoid, []Code{
		Linef("entity := req.Msg.%s", m.EntityName),
		If("entity == nil", Return(fmt.Sprintf("nil, connect.NewError(connect.CodeInvalidArgument, errors.New(\"%s is required\"))", entity))),
		Blank(),
		Linef("id, err := s.%sRepo.Create(ctx, entity)", entity),
		If("err != nil", Concat(CodeMonoid, []Code{
			If("errors.Is(err, ErrAlreadyExists)", Return("nil, connect.NewError(connect.CodeAlreadyExists, err)")),
			Return("nil, connect.NewError(connect.CodeInternal, err)"),
		})),
		Blank(),
		Linef("created, err := s.%sRepo.Get(ctx, id)", entity),
		If("err != nil", Return("nil, connect.NewError(connect.CodeInternal, err)")),
		Blank(),
		Linef("return connect.NewResponse(&%s{%s: created}), nil", m.OutType, m.EntityName),
	})
}

func getMethodBody(m MethodInfo) Code {
	entity := lowerFirst(m.EntityName)
	return Concat(CodeMonoid, []Code{
		Line("id := req.Msg.Id"),
		If(`id == ""`, Return("nil, connect.NewError(connect.CodeInvalidArgument, errors.New(\"id is required\"))")),
		Blank(),
		Linef("entity, err := s.%sRepo.Get(ctx, id)", entity),
		If("err != nil", Concat(CodeMonoid, []Code{
			If("errors.Is(err, ErrNotFound)", Return("nil, connect.NewError(connect.CodeNotFound, err)")),
			Return("nil, connect.NewError(connect.CodeInternal, err)"),
		})),
		Blank(),
		Linef("return connect.NewResponse(&%s{%s: entity}), nil", m.OutType, m.EntityName),
	})
}

func listMethodBody(m MethodInfo) Code {
	entity := lowerFirst(m.EntityName)
	return Concat(CodeMonoid, []Code{
		Comment("Pagination can be added via query builder if needed"),
		Line("_ = req.Msg // Request may have pagination params"),
		Blank(),
		Linef("entities, err := s.%sRepo.List(ctx)", entity),
		If("err != nil", Return("nil, connect.NewError(connect.CodeInternal, err)")),
		Blank(),
		Linef("return connect.NewResponse(&%s{%ss: entities}), nil", m.OutType, m.EntityName),
	})
}

func updateMethodBody(m MethodInfo) Code {
	entity := lowerFirst(m.EntityName)
	return Concat(CodeMonoid, []Code{
		Linef("entity := req.Msg.%s", m.EntityName),
		If("entity == nil", Return(fmt.Sprintf("nil, connect.NewError(connect.CodeInvalidArgument, errors.New(\"%s is required\"))", entity))),
		If(`entity.Id == ""`, Return("nil, connect.NewError(connect.CodeInvalidArgument, errors.New(\"id is required\"))")),
		Blank(),
		Linef("err := s.%sRepo.Update(ctx, entity)", entity),
		If("err != nil", Concat(CodeMonoid, []Code{
			If("errors.Is(err, ErrNotFound)", Return("nil, connect.NewError(connect.CodeNotFound, err)")),
			Return("nil, connect.NewError(connect.CodeInternal, err)"),
		})),
		Blank(),
		Linef("updated, err := s.%sRepo.Get(ctx, entity.Id)", entity),
		If("err != nil", Return("nil, connect.NewError(connect.CodeInternal, err)")),
		Blank(),
		Linef("return connect.NewResponse(&%s{%s: updated}), nil", m.OutType, m.EntityName),
	})
}

func deleteMethodBody(m MethodInfo) Code {
	entity := lowerFirst(m.EntityName)
	return Concat(CodeMonoid, []Code{
		Line("id := req.Msg.Id"),
		If(`id == ""`, Return("nil, connect.NewError(connect.CodeInvalidArgument, errors.New(\"id is required\"))")),
		Blank(),
		Linef("err := s.%sRepo.Delete(ctx, id)", entity),
		If("err != nil", Concat(CodeMonoid, []Code{
			If("errors.Is(err, ErrNotFound)", Return("nil, connect.NewError(connect.CodeNotFound, err)")),
			Return("nil, connect.NewError(connect.CodeInternal, err)"),
		})),
		Blank(),
		Linef("return connect.NewResponse(&%s{}), nil", m.OutType),
	})
}

func softDeleteMethodBody(m MethodInfo) Code {
	entity := lowerFirst(m.EntityName)
	return Concat(CodeMonoid, []Code{
		Line("id := req.Msg.Id"),
		If(`id == ""`, Return("nil, connect.NewError(connect.CodeInvalidArgument, errors.New(\"id is required\"))")),
		Blank(),
		Comment("Get entity, set deleted_at, then update"),
		Linef("existing, err := s.%sRepo.Get(ctx, id)", entity),
		If("err != nil", Concat(CodeMonoid, []Code{
			If("errors.Is(err, ErrNotFound)", Return("nil, connect.NewError(connect.CodeNotFound, err)")),
			Return("nil, connect.NewError(connect.CodeInternal, err)"),
		})),
		Line("existing.DeletedAt = timestamppb.Now()"),
		Linef("if err := s.%sRepo.Update(ctx, existing); err != nil {", entity),
		Line("\treturn nil, connect.NewError(connect.CodeInternal, err)"),
		Line("}"),
		Blank(),
		Linef("return connect.NewResponse(&%s{}), nil", m.OutType),
	})
}

func restoreMethodBody(m MethodInfo) Code {
	entity := lowerFirst(m.EntityName)
	return Concat(CodeMonoid, []Code{
		Line("id := req.Msg.Id"),
		If(`id == ""`, Return("nil, connect.NewError(connect.CodeInvalidArgument, errors.New(\"id is required\"))")),
		Blank(),
		Comment("Get entity, clear deleted_at, then update"),
		Linef("existing, err := s.%sRepo.Get(ctx, id)", entity),
		If("err != nil", Concat(CodeMonoid, []Code{
			If("errors.Is(err, ErrNotFound)", Return("nil, connect.NewError(connect.CodeNotFound, err)")),
			Return("nil, connect.NewError(connect.CodeInternal, err)"),
		})),
		Line("existing.DeletedAt = nil"),
		Linef("if err := s.%sRepo.Update(ctx, existing); err != nil {", entity),
		Line("\treturn nil, connect.NewError(connect.CodeInternal, err)"),
		Line("}"),
		Blank(),
		Linef("return connect.NewResponse(&%s{%s: existing}), nil", m.OutType, m.EntityName),
	})
}

func customMethodBody(m MethodInfo) Code {
	return Concat(CodeMonoid, []Code{
		Comment("TODO: Implement custom method logic"),
		Linef("return nil, connect.NewError(connect.CodeUnimplemented, errors.New(\"%s not implemented\"))", m.GoName),
	})
}

func RepositoryInterface(m MessageInfo) Code {
	// Interface is defined in firestore plugin
	return CodeMonoid.Empty()
}

func ServerMain() Code {
	return Concat(CodeMonoid, []Code{
		Blank(),
		Comment("============================================================================="),
		Comment("SERVER SETUP HELPERS"),
		Comment("============================================================================="),
		Blank(),
		Comment("ServerConfig holds server configuration"),
		Struct("ServerConfig", Concat(CodeMonoid, []Code{
			Field("Port", "int"),
			Field("Host", "string"),
			Field("EnableCORS", "bool"),
			Field("EnableReflection", "bool"),
		})),
		Blank(),
		Comment("DefaultServerConfig returns sensible defaults"),
		Func("DefaultServerConfig", "", "ServerConfig",
			Return("ServerConfig{Port: 8080, Host: \"0.0.0.0\", EnableCORS: true, EnableReflection: true}")),
		Blank(),
		Comment("NewServeMux creates a new HTTP mux with standard middleware"),
		Func("NewServeMux", "", "*http.ServeMux",
			Return("http.NewServeMux()")),
		Blank(),
		Comment("WithCORS wraps handler with CORS middleware"),
		Func("WithCORS", "h http.Handler", "http.Handler",
			Concat(CodeMonoid, []Code{
				Line("return cors.New(cors.Options{"),
				Line("\tAllowedOrigins: []string{\"*\"},"),
				Line("\tAllowedMethods: []string{\"GET\", \"POST\", \"PUT\", \"DELETE\", \"OPTIONS\"},"),
				Line("\tAllowedHeaders: []string{\"Accept\", \"Authorization\", \"Content-Type\", \"Connect-Protocol-Version\"},"),
				Line("\tExposedHeaders: []string{\"Grpc-Status\", \"Grpc-Message\"},"),
				Line("\tAllowCredentials: true,"),
				Line("}).Handler(h)"),
			})),
		Blank(),
		Comment("RunServer starts the HTTP server"),
		Func("RunServer", "cfg ServerConfig, mux *http.ServeMux", "error",
			Concat(CodeMonoid, []Code{
				Line("addr := fmt.Sprintf(\"%s:%d\", cfg.Host, cfg.Port)"),
				Blank(),
				Line("var handler http.Handler = mux"),
				If("cfg.EnableCORS", Line("handler = WithCORS(handler)")),
				Blank(),
				Line("server := &http.Server{"),
				Line("\tAddr:         addr,"),
				Line("\tHandler:      handler,"),
				Line("\tReadTimeout:  30 * time.Second,"),
				Line("\tWriteTimeout: 30 * time.Second,"),
				Line("}"),
				Blank(),
				Line(`log.Printf("Starting server on %s", addr)`),
				Return("server.ListenAndServe()"),
			})),
	})
}

func ServiceRegistration(s ServiceInfo, pkgName string) Code {
	return Concat(CodeMonoid, []Code{
		Blank(),
		Commentf("Register%sServer registers the %s service with the mux", s.GoName, s.GoName),
		Commentf("Service path: /%s.%s/", pkgName, s.GoName),
		Func("Register"+s.GoName+"Server", fmt.Sprintf("mux *http.ServeMux, server *%sServer", s.GoName), "",
			Concat(CodeMonoid, []Code{
				Linef("path := \"/%s.%s/\"", pkgName, s.GoName),
				Line("mux.Handle(path, server)"),
			})),
	})
}

func ServiceHTTPHandler(s ServiceInfo) Code {
	// Generate ServeHTTP method for the server struct
	cases := FoldMap(s.Methods, CodeMonoid, func(m MethodInfo) Code {
		return Concat(CodeMonoid, []Code{
			Linef(`case "%s":`, m.GoName),
			Linef("\ts.handle%s(w, r)", m.GoName),
		})
	})

	return Concat(CodeMonoid, []Code{
		Blank(),
		Comment("ServeHTTP implements http.Handler for Connect-style routing"),
		Method("s *"+s.GoName+"Server", "ServeHTTP", "w http.ResponseWriter, r *http.Request", "",
			Concat(CodeMonoid, []Code{
				Line("// Extract method name from URL path"),
				Line(`parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")`),
				If("len(parts) < 2", Concat(CodeMonoid, []Code{
					Line("http.Error(w, \"invalid path\", http.StatusBadRequest)"),
					Return(),
				})),
				Line("method := parts[len(parts)-1]"),
				Blank(),
				Line("switch method {"),
				cases,
				Line("default:"),
				Line("\thttp.Error(w, \"method not found\", http.StatusNotFound)"),
				Line("}"),
			})),
	})
}

func ServiceMethodHTTPHandler(s ServiceInfo, m MethodInfo) Code {
	return Concat(CodeMonoid, []Code{
		Blank(),
		Commentf("handle%s handles HTTP requests for %s", m.GoName, m.GoName),
		Method("s *"+s.GoName+"Server", "handle"+m.GoName, "w http.ResponseWriter, r *http.Request", "",
			Concat(CodeMonoid, []Code{
				Line("ctx := r.Context()"),
				Blank(),
				Comment("Decode request"),
				Linef("var req %s", m.InputType),
				Line("if err := json.NewDecoder(r.Body).Decode(&req); err != nil {"),
				Line("\thttp.Error(w, err.Error(), http.StatusBadRequest)"),
				Line("\treturn"),
				Line("}"),
				Blank(),
				Comment("Create connect-style request wrapper"),
				Linef("connectReq := &connect.Request[%s]{Msg: &req}", m.InputType),
				Blank(),
				Comment("Call the service method"),
				Linef("resp, err := s.%s(ctx, connectReq)", m.GoName),
				Line("if err != nil {"),
				Line("\tvar connectErr *connect.Error"),
				Line("\tif errors.As(err, &connectErr) {"),
				Line("\t\thttp.Error(w, connectErr.Message(), int(connectErr.Code()))"),
				Line("\t} else {"),
				Line("\t\thttp.Error(w, err.Error(), http.StatusInternalServerError)"),
				Line("\t}"),
				Line("\treturn"),
				Line("}"),
				Blank(),
				Comment("Encode response"),
				Line("w.Header().Set(\"Content-Type\", \"application/json\")"),
				Line("json.NewEncoder(w).Encode(resp.Msg)"),
			})),
	})
}

func GenerateService(s ServiceInfo, messages []MessageInfo, pkgName string) Code {
	return Concat(CodeMonoid, []Code{
		ServiceStruct(s, messages),
		ServiceConstructor(s),
		FoldMap(s.Methods, CodeMonoid, func(m MethodInfo) Code {
			return ServiceMethod(s, m)
		}),
		ServiceHTTPHandler(s),
		FoldMap(s.Methods, CodeMonoid, func(m MethodInfo) Code {
			return ServiceMethodHTTPHandler(s, m)
		}),
		ServiceRegistration(s, pkgName),
	})
}

func GenerateFile(file *protogen.File) Code {
	if len(file.Services) == 0 {
		return CodeMonoid.Empty()
	}

	messages := Map(file.Messages, ExtractMessageInfo)
	services := Map(file.Services, ExtractServiceInfo)
	pkgName := string(file.GoPackageName)

	// Import path for the proto package (where messages are defined)
	protoImportPath := string(file.GoImportPath)

	return Concat(CodeMonoid, []Code{
		Header(), Blank(),
		Package(pkgName),
		Imports(
			"context", "encoding/json", "errors", "fmt", "log", "net/http", "strings", "time", "",
			"connectrpc.com/connect",
			"github.com/rs/cors",
			"google.golang.org/protobuf/types/known/timestamppb",
		),
		Blank(),
		Comment("============================================================================="),
		Comment("REPOSITORY INTERFACES (same package - no import needed)"),
		Comment("============================================================================="),
		FoldMap(messages, CodeMonoid, RepositoryInterface),
		Blank(),
		Comment("============================================================================="),
		Comment("CONNECT SERVICE IMPLEMENTATIONS"),
		Comment("============================================================================="),
		Commentf("Proto import path: %s", protoImportPath),
		FoldMap(services, CodeMonoid, func(s ServiceInfo) Code {
			return GenerateService(s, messages, pkgName)
		}),
		ServerMain(),
	})
}

func main() {
	protogen.Options{}.Run(func(gen *protogen.Plugin) error {
		gen.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)
		for _, f := range gen.Files {
			if !f.Generate || len(f.Services) == 0 {
				continue
			}
			g := gen.NewGeneratedFile(f.GeneratedFilenamePrefix+"_connect_server.pb.go", f.GoImportPath)
			g.P(GenerateFile(f).Run())
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

func toSnakeCase(s string) string {
	var result []rune
	for i, r := range s {
		if i > 0 && unicode.IsUpper(r) {
			result = append(result, '_')
		}
		result = append(result, unicode.ToLower(r))
	}
	return string(result)
}
