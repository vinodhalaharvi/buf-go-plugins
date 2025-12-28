// protoc-gen-openapi generates OpenAPI 3.0 specification from protobuf
// Uses Category Theory: Monoid + Functor + Fold
package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoreflect"
	pluginpb "google.golang.org/protobuf/types/pluginpb"
)

// =============================================================================
// OPENAPI TYPES
// =============================================================================

type OpenAPI struct {
	OpenAPI    string              `json:"openapi"`
	Info       Info                `json:"info"`
	Servers    []Server            `json:"servers,omitempty"`
	Paths      map[string]PathItem `json:"paths"`
	Components Components          `json:"components"`
	Tags       []Tag               `json:"tags,omitempty"`
}

type Info struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version"`
}

type Server struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

type Tag struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type PathItem struct {
	Get    *Operation `json:"get,omitempty"`
	Post   *Operation `json:"post,omitempty"`
	Put    *Operation `json:"put,omitempty"`
	Delete *Operation `json:"delete,omitempty"`
	Patch  *Operation `json:"patch,omitempty"`
}

type Operation struct {
	Tags        []string            `json:"tags,omitempty"`
	Summary     string              `json:"summary,omitempty"`
	Description string              `json:"description,omitempty"`
	OperationID string              `json:"operationId,omitempty"`
	Parameters  []Parameter         `json:"parameters,omitempty"`
	RequestBody *RequestBody        `json:"requestBody,omitempty"`
	Responses   map[string]Response `json:"responses"`
	Security    []SecurityReq       `json:"security,omitempty"`
}

type Parameter struct {
	Name        string  `json:"name"`
	In          string  `json:"in"` // query, path, header, cookie
	Description string  `json:"description,omitempty"`
	Required    bool    `json:"required,omitempty"`
	Schema      *Schema `json:"schema,omitempty"`
}

type RequestBody struct {
	Description string               `json:"description,omitempty"`
	Required    bool                 `json:"required,omitempty"`
	Content     map[string]MediaType `json:"content"`
}

type Response struct {
	Description string               `json:"description"`
	Content     map[string]MediaType `json:"content,omitempty"`
}

type MediaType struct {
	Schema *Schema `json:"schema,omitempty"`
}

type Schema struct {
	Type        string             `json:"type,omitempty"`
	Format      string             `json:"format,omitempty"`
	Description string             `json:"description,omitempty"`
	Properties  map[string]*Schema `json:"properties,omitempty"`
	Items       *Schema            `json:"items,omitempty"`
	Required    []string           `json:"required,omitempty"`
	Ref         string             `json:"$ref,omitempty"`
	Enum        []string           `json:"enum,omitempty"`
	Example     interface{}        `json:"example,omitempty"`
}

type Components struct {
	Schemas         map[string]*Schema        `json:"schemas,omitempty"`
	SecuritySchemes map[string]SecurityScheme `json:"securitySchemes,omitempty"`
}

type SecurityScheme struct {
	Type         string `json:"type"`
	Scheme       string `json:"scheme,omitempty"`
	BearerFormat string `json:"bearerFormat,omitempty"`
	Name         string `json:"name,omitempty"`
	In           string `json:"in,omitempty"`
}

type SecurityReq map[string][]string

// =============================================================================
// GENERATOR
// =============================================================================

func protoKindToOpenAPI(kind protoreflect.Kind) (string, string) {
	switch kind {
	case protoreflect.BoolKind:
		return "boolean", ""
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return "integer", "int32"
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return "integer", "int64"
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return "integer", "int32"
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return "integer", "int64"
	case protoreflect.FloatKind:
		return "number", "float"
	case protoreflect.DoubleKind:
		return "number", "double"
	case protoreflect.StringKind:
		return "string", ""
	case protoreflect.BytesKind:
		return "string", "byte"
	default:
		return "string", ""
	}
}

func GenerateOpenAPI(file *protogen.File) *OpenAPI {
	pkgName := string(file.GoPackageName)

	spec := &OpenAPI{
		OpenAPI: "3.0.3",
		Info: Info{
			Title:       pkgName + " API",
			Description: "Auto-generated REST API from protobuf definitions",
			Version:     "1.0.0",
		},
		Servers: []Server{
			{URL: "http://localhost:8080", Description: "Local development"},
			{URL: "https://api.example.com", Description: "Production"},
		},
		Paths: make(map[string]PathItem),
		Components: Components{
			Schemas: make(map[string]*Schema),
			SecuritySchemes: map[string]SecurityScheme{
				"bearerAuth": {
					Type:         "http",
					Scheme:       "bearer",
					BearerFormat: "JWT",
				},
			},
		},
		Tags: []Tag{},
	}

	// Generate schemas for each message
	for _, msg := range file.Messages {
		spec.Components.Schemas[msg.GoIdent.GoName] = generateSchema(msg)
		spec.Components.Schemas[msg.GoIdent.GoName+"Input"] = generateInputSchema(msg)
		spec.Tags = append(spec.Tags, Tag{
			Name:        msg.GoIdent.GoName,
			Description: fmt.Sprintf("Operations for %s resources", msg.GoIdent.GoName),
		})
	}

	// Generate paths for each message (RESTful CRUD)
	for _, msg := range file.Messages {
		name := msg.GoIdent.GoName
		lower := strings.ToLower(name)
		basePath := "/" + lower + "s"

		// List and Create
		spec.Paths[basePath] = PathItem{
			Get:  generateListOperation(msg),
			Post: generateCreateOperation(msg),
		}

		// Get, Update, Delete
		spec.Paths[basePath+"/{id}"] = PathItem{
			Get:    generateGetOperation(msg),
			Put:    generateUpdateOperation(msg),
			Delete: generateDeleteOperation(msg),
			Patch:  generatePatchOperation(msg),
		}
	}

	// Generate paths for services
	for _, svc := range file.Services {
		for _, method := range svc.Methods {
			path := "/" + strings.ToLower(string(svc.Desc.Name())) + "/" + strings.ToLower(string(method.Desc.Name()))
			spec.Paths[path] = PathItem{
				Post: generateRPCOperation(svc, method),
			}
		}
	}

	return spec
}

func generateSchema(msg *protogen.Message) *Schema {
	schema := &Schema{
		Type:       "object",
		Properties: make(map[string]*Schema),
		Required:   []string{},
	}

	for _, field := range msg.Fields {
		fieldName := string(field.Desc.Name())
		fieldSchema := fieldToSchema(field)
		schema.Properties[fieldName] = fieldSchema

		// ID is always required
		if fieldName == "id" {
			schema.Required = append(schema.Required, fieldName)
		}
	}

	return schema
}

func generateInputSchema(msg *protogen.Message) *Schema {
	schema := &Schema{
		Type:       "object",
		Properties: make(map[string]*Schema),
	}

	for _, field := range msg.Fields {
		fieldName := string(field.Desc.Name())
		if fieldName == "id" {
			continue // Skip ID in input
		}
		schema.Properties[fieldName] = fieldToSchema(field)
	}

	return schema
}

func fieldToSchema(field *protogen.Field) *Schema {
	// Handle message types
	if field.Message != nil {
		msgName := field.Message.GoIdent.GoName

		// Timestamp
		if strings.Contains(msgName, "Timestamp") {
			return &Schema{Type: "string", Format: "date-time"}
		}

		// Nested message
		if field.Desc.IsList() {
			return &Schema{
				Type:  "array",
				Items: &Schema{Ref: "#/components/schemas/" + msgName},
			}
		}
		return &Schema{Ref: "#/components/schemas/" + msgName}
	}

	// Handle enum types
	if field.Enum != nil {
		var values []string
		for _, v := range field.Enum.Values {
			values = append(values, string(v.Desc.Name()))
		}
		return &Schema{Type: "string", Enum: values}
	}

	// Handle scalar types
	typ, format := protoKindToOpenAPI(field.Desc.Kind())

	if field.Desc.IsList() {
		return &Schema{
			Type:  "array",
			Items: &Schema{Type: typ, Format: format},
		}
	}

	schema := &Schema{Type: typ}
	if format != "" {
		schema.Format = format
	}
	return schema
}

func generateListOperation(msg *protogen.Message) *Operation {
	name := msg.GoIdent.GoName
	return &Operation{
		Tags:        []string{name},
		Summary:     "List " + name + " resources",
		OperationID: "list" + name,
		Parameters: []Parameter{
			{Name: "limit", In: "query", Description: "Maximum number of results", Schema: &Schema{Type: "integer", Format: "int32"}},
			{Name: "offset", In: "query", Description: "Number of results to skip", Schema: &Schema{Type: "integer", Format: "int32"}},
			{Name: "sort", In: "query", Description: "Sort field", Schema: &Schema{Type: "string"}},
			{Name: "order", In: "query", Description: "Sort order (asc/desc)", Schema: &Schema{Type: "string", Enum: []string{"asc", "desc"}}},
		},
		Responses: map[string]Response{
			"200": {
				Description: "Successful response",
				Content: map[string]MediaType{
					"application/json": {
						Schema: &Schema{
							Type:  "array",
							Items: &Schema{Ref: "#/components/schemas/" + name},
						},
					},
				},
			},
			"401": {Description: "Unauthorized"},
			"500": {Description: "Internal server error"},
		},
		Security: []SecurityReq{{"bearerAuth": {}}},
	}
}

func generateCreateOperation(msg *protogen.Message) *Operation {
	name := msg.GoIdent.GoName
	return &Operation{
		Tags:        []string{name},
		Summary:     "Create a new " + name,
		OperationID: "create" + name,
		RequestBody: &RequestBody{
			Required: true,
			Content: map[string]MediaType{
				"application/json": {
					Schema: &Schema{Ref: "#/components/schemas/" + name + "Input"},
				},
			},
		},
		Responses: map[string]Response{
			"201": {
				Description: "Created",
				Content: map[string]MediaType{
					"application/json": {
						Schema: &Schema{Ref: "#/components/schemas/" + name},
					},
				},
			},
			"400": {Description: "Bad request"},
			"401": {Description: "Unauthorized"},
			"500": {Description: "Internal server error"},
		},
		Security: []SecurityReq{{"bearerAuth": {}}},
	}
}

func generateGetOperation(msg *protogen.Message) *Operation {
	name := msg.GoIdent.GoName
	return &Operation{
		Tags:        []string{name},
		Summary:     "Get a " + name + " by ID",
		OperationID: "get" + name,
		Parameters: []Parameter{
			{Name: "id", In: "path", Required: true, Description: name + " ID", Schema: &Schema{Type: "string"}},
		},
		Responses: map[string]Response{
			"200": {
				Description: "Successful response",
				Content: map[string]MediaType{
					"application/json": {
						Schema: &Schema{Ref: "#/components/schemas/" + name},
					},
				},
			},
			"401": {Description: "Unauthorized"},
			"404": {Description: "Not found"},
			"500": {Description: "Internal server error"},
		},
		Security: []SecurityReq{{"bearerAuth": {}}},
	}
}

func generateUpdateOperation(msg *protogen.Message) *Operation {
	name := msg.GoIdent.GoName
	return &Operation{
		Tags:        []string{name},
		Summary:     "Update a " + name,
		OperationID: "update" + name,
		Parameters: []Parameter{
			{Name: "id", In: "path", Required: true, Description: name + " ID", Schema: &Schema{Type: "string"}},
		},
		RequestBody: &RequestBody{
			Required: true,
			Content: map[string]MediaType{
				"application/json": {
					Schema: &Schema{Ref: "#/components/schemas/" + name + "Input"},
				},
			},
		},
		Responses: map[string]Response{
			"200": {
				Description: "Updated",
				Content: map[string]MediaType{
					"application/json": {
						Schema: &Schema{Ref: "#/components/schemas/" + name},
					},
				},
			},
			"400": {Description: "Bad request"},
			"401": {Description: "Unauthorized"},
			"404": {Description: "Not found"},
			"500": {Description: "Internal server error"},
		},
		Security: []SecurityReq{{"bearerAuth": {}}},
	}
}

func generatePatchOperation(msg *protogen.Message) *Operation {
	name := msg.GoIdent.GoName
	return &Operation{
		Tags:        []string{name},
		Summary:     "Partially update a " + name,
		OperationID: "patch" + name,
		Parameters: []Parameter{
			{Name: "id", In: "path", Required: true, Description: name + " ID", Schema: &Schema{Type: "string"}},
		},
		RequestBody: &RequestBody{
			Required: true,
			Content: map[string]MediaType{
				"application/json": {
					Schema: &Schema{Ref: "#/components/schemas/" + name + "Input"},
				},
			},
		},
		Responses: map[string]Response{
			"200": {
				Description: "Updated",
				Content: map[string]MediaType{
					"application/json": {
						Schema: &Schema{Ref: "#/components/schemas/" + name},
					},
				},
			},
			"400": {Description: "Bad request"},
			"401": {Description: "Unauthorized"},
			"404": {Description: "Not found"},
			"500": {Description: "Internal server error"},
		},
		Security: []SecurityReq{{"bearerAuth": {}}},
	}
}

func generateDeleteOperation(msg *protogen.Message) *Operation {
	name := msg.GoIdent.GoName
	return &Operation{
		Tags:        []string{name},
		Summary:     "Delete a " + name,
		OperationID: "delete" + name,
		Parameters: []Parameter{
			{Name: "id", In: "path", Required: true, Description: name + " ID", Schema: &Schema{Type: "string"}},
		},
		Responses: map[string]Response{
			"204": {Description: "Deleted"},
			"401": {Description: "Unauthorized"},
			"404": {Description: "Not found"},
			"500": {Description: "Internal server error"},
		},
		Security: []SecurityReq{{"bearerAuth": {}}},
	}
}

func generateRPCOperation(svc *protogen.Service, method *protogen.Method) *Operation {
	return &Operation{
		Tags:        []string{string(svc.Desc.Name())},
		Summary:     string(method.Desc.Name()),
		OperationID: string(svc.Desc.Name()) + "_" + string(method.Desc.Name()),
		RequestBody: &RequestBody{
			Required: true,
			Content: map[string]MediaType{
				"application/json": {
					Schema: &Schema{Ref: "#/components/schemas/" + method.Input.GoIdent.GoName},
				},
			},
		},
		Responses: map[string]Response{
			"200": {
				Description: "Successful response",
				Content: map[string]MediaType{
					"application/json": {
						Schema: &Schema{Ref: "#/components/schemas/" + method.Output.GoIdent.GoName},
					},
				},
			},
			"400": {Description: "Bad request"},
			"401": {Description: "Unauthorized"},
			"500": {Description: "Internal server error"},
		},
		Security: []SecurityReq{{"bearerAuth": {}}},
	}
}

// =============================================================================
// REST HANDLER GENERATOR
// =============================================================================

func GenerateRESTHandlers(pkgName string, file *protogen.File) string {
	var sb strings.Builder

	sb.WriteString("// Code generated by protoc-gen-openapi. DO NOT EDIT.\n")
	sb.WriteString(fmt.Sprintf("package %s\n\n", pkgName))

	sb.WriteString(`import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

`)

	// Generate handler struct
	sb.WriteString("type RESTHandler struct {\n")
	for _, msg := range file.Messages {
		sb.WriteString(fmt.Sprintf("\t%sRepo %sRepository\n", msg.GoIdent.GoName, msg.GoIdent.GoName))
	}
	sb.WriteString("}\n\n")

	// Generate router setup
	sb.WriteString("func (h *RESTHandler) RegisterRoutes(mux *http.ServeMux) {\n")
	for _, msg := range file.Messages {
		lower := strings.ToLower(msg.GoIdent.GoName)
		sb.WriteString(fmt.Sprintf("\tmux.HandleFunc(\"/%ss\", h.handle%ss)\n", lower, msg.GoIdent.GoName))
		sb.WriteString(fmt.Sprintf("\tmux.HandleFunc(\"/%ss/\", h.handle%s)\n", lower, msg.GoIdent.GoName))
	}
	sb.WriteString("}\n\n")

	// Generate handlers for each message
	for _, msg := range file.Messages {
		sb.WriteString(generateMessageHandlers(msg))
	}

	// Helper functions
	sb.WriteString(`func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func getIDFromPath(path, prefix string) string {
	return strings.TrimPrefix(path, prefix)
}
`)

	return sb.String()
}

func generateMessageHandlers(msg *protogen.Message) string {
	name := msg.GoIdent.GoName
	lower := strings.ToLower(name)

	return fmt.Sprintf(`// %s collection handler (GET list, POST create)
func (h *RESTHandler) handle%ss(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := h.%sRepo.List(r.Context())
		if err != nil { writeError(w, 500, err.Error()); return }
		
		// Pagination
		limit := 100
		offset := 0
		if l := r.URL.Query().Get("limit"); l != "" { limit, _ = strconv.Atoi(l) }
		if o := r.URL.Query().Get("offset"); o != "" { offset, _ = strconv.Atoi(o) }
		
		end := offset + limit
		if end > len(items) { end = len(items) }
		if offset > len(items) { offset = len(items) }
		
		writeJSON(w, 200, items[offset:end])

	case http.MethodPost:
		var input %s
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil { writeError(w, 400, "invalid JSON"); return }
		
		id, err := h.%sRepo.Create(r.Context(), &input)
		if err != nil { writeError(w, 500, err.Error()); return }
		input.Id = id
		
		writeJSON(w, 201, input)

	default:
		writeError(w, 405, "method not allowed")
	}
}

// %s single resource handler (GET, PUT, PATCH, DELETE)
func (h *RESTHandler) handle%s(w http.ResponseWriter, r *http.Request) {
	id := getIDFromPath(r.URL.Path, "/%ss/")
	if id == "" { writeError(w, 400, "missing id"); return }

	switch r.Method {
	case http.MethodGet:
		item, err := h.%sRepo.Get(r.Context(), id)
		if err != nil { writeError(w, 404, "not found"); return }
		writeJSON(w, 200, item)

	case http.MethodPut, http.MethodPatch:
		var input %s
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil { writeError(w, 400, "invalid JSON"); return }
		input.Id = id
		
		if err := h.%sRepo.Update(r.Context(), &input); err != nil { writeError(w, 500, err.Error()); return }
		writeJSON(w, 200, input)

	case http.MethodDelete:
		if err := h.%sRepo.Delete(r.Context(), id); err != nil { writeError(w, 500, err.Error()); return }
		w.WriteHeader(204)

	default:
		writeError(w, 405, "method not allowed")
	}
}

`, name, name, name, name, name, name, name, lower, name, name, name, name)
}

// =============================================================================
// MAIN
// =============================================================================

func main() {
	protogen.Options{}.Run(func(gen *protogen.Plugin) error {
		gen.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)
		for _, f := range gen.Files {
			if !f.Generate || len(f.Messages) == 0 {
				continue
			}

			pkgName := string(f.GoPackageName)

			// Generate OpenAPI spec (JSON)
			spec := GenerateOpenAPI(f)
			specJSON, _ := json.MarshalIndent(spec, "", "  ")

			jsonFile := gen.NewGeneratedFile(f.GeneratedFilenamePrefix+"_openapi.json", "")
			jsonFile.P(string(specJSON))

			// Generate Go REST handlers
			handlersFile := gen.NewGeneratedFile(f.GeneratedFilenamePrefix+"_rest_handlers.pb.go", f.GoImportPath)
			handlersFile.P(GenerateRESTHandlers(pkgName, f))
		}
		return nil
	})
}
