// protoc-gen-llm generates business logic using Claude AI
// Reads @llm: comments from proto, sends to Claude API, generates Go logic layer
// Uses Category Theory: Monoid + Functor + Fold
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"google.golang.org/protobuf/compiler/protogen"
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

func Line(s string) Code                            { return Code{Run: func() string { return s + "\n" }} }
func Linef(format string, args ...interface{}) Code { return Line(fmt.Sprintf(format, args...)) }
func Blank() Code                                   { return Line("") }

// =============================================================================
// LLM DIRECTIVE EXTRACTION
// =============================================================================

type LLMDirective struct {
	ServiceName string
	MethodName  string
	InputType   string
	OutputType  string
	Directive   string // The @llm: comment content
}

type ServiceInfo struct {
	Name       string
	Methods    []MethodInfo
	Directives []LLMDirective
}

type MethodInfo struct {
	Name       string
	InputType  string
	OutputType string
	Comments   string
}

var llmRegex = regexp.MustCompile(`@llm:\s*(.+)`)

func ExtractLLMDirectives(service *protogen.Service) []LLMDirective {
	var directives []LLMDirective

	for _, method := range service.Methods {
		// Get comments
		comments := string(method.Comments.Leading)

		// Find @llm: directive
		matches := llmRegex.FindStringSubmatch(comments)
		if len(matches) > 1 {
			directive := strings.TrimSpace(matches[1])
			// Handle multi-line: collect everything after @llm:
			lines := strings.Split(comments, "\n")
			var fullDirective []string
			capturing := false
			for _, line := range lines {
				line = strings.TrimPrefix(line, "//")
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "@llm:") {
					capturing = true
					line = strings.TrimPrefix(line, "@llm:")
					line = strings.TrimSpace(line)
				}
				if capturing && line != "" {
					fullDirective = append(fullDirective, line)
				}
			}
			if len(fullDirective) > 0 {
				directive = strings.Join(fullDirective, " ")
			}

			directives = append(directives, LLMDirective{
				ServiceName: service.GoName,
				MethodName:  method.GoName,
				InputType:   method.Input.GoIdent.GoName,
				OutputType:  method.Output.GoIdent.GoName,
				Directive:   directive,
			})
		}
	}

	return directives
}

// =============================================================================
// PROTO EXTRACTION (for Claude context)
// =============================================================================

func ExtractProtoContext(file *protogen.File) string {
	var sb strings.Builder

	// Messages
	sb.WriteString("// Message definitions:\n")
	for _, msg := range file.Messages {
		sb.WriteString(fmt.Sprintf("message %s {\n", msg.GoIdent.GoName))
		for _, field := range msg.Fields {
			sb.WriteString(fmt.Sprintf("  %s %s = %d;\n",
				fieldTypeName(field), field.Desc.Name(), field.Desc.Number()))
		}
		sb.WriteString("}\n\n")
	}

	// Enums
	for _, enum := range file.Enums {
		sb.WriteString(fmt.Sprintf("enum %s {\n", enum.GoIdent.GoName))
		for _, value := range enum.Values {
			sb.WriteString(fmt.Sprintf("  %s = %d;\n", value.Desc.Name(), value.Desc.Number()))
		}
		sb.WriteString("}\n\n")
	}

	// Services with comments
	for _, svc := range file.Services {
		sb.WriteString(fmt.Sprintf("service %s {\n", svc.GoName))
		for _, method := range svc.Methods {
			comments := strings.TrimSpace(string(method.Comments.Leading))
			if comments != "" {
				for _, line := range strings.Split(comments, "\n") {
					sb.WriteString(fmt.Sprintf("  //%s\n", line))
				}
			}
			sb.WriteString(fmt.Sprintf("  rpc %s(%s) returns (%s);\n",
				method.GoName, method.Input.GoIdent.GoName, method.Output.GoIdent.GoName))
		}
		sb.WriteString("}\n\n")
	}

	return sb.String()
}

func fieldTypeName(field *protogen.Field) string {
	switch field.Desc.Kind().String() {
	case "message":
		return field.Message.GoIdent.GoName
	case "enum":
		return field.Enum.GoIdent.GoName
	default:
		return field.Desc.Kind().String()
	}
}

// =============================================================================
// PB.GO FILE READING (for exact Go types)
// =============================================================================

func ReadGeneratedPbFiles(file *protogen.File) string {
	var sb strings.Builder

	// Construct paths to generated .pb.go files
	// These are typically in gen/go/<package>/<version>/
	basePath := os.Getenv("GEN_PATH")
	if basePath == "" {
		basePath = "gen/go"
	}

	// Get the proto file path to derive the pb.go path
	protoPath := file.Desc.Path()
	// Convert proto/example/v1/models.proto -> gen/go/example/v1/models.pb.go
	pbPath := strings.TrimSuffix(protoPath, ".proto") + ".pb.go"
	fullPath := fmt.Sprintf("%s/%s", basePath, strings.TrimPrefix(pbPath, "proto/"))

	// Try to read the models.pb.go
	if content, err := os.ReadFile(fullPath); err == nil {
		sb.WriteString("// ============================================\n")
		sb.WriteString("// Generated Go types from models.pb.go:\n")
		sb.WriteString("// ============================================\n\n")
		sb.WriteString(extractTypeDefinitions(string(content)))
		sb.WriteString("\n\n")
	}

	// Also try to read service.pb.go for request/response types
	servicePbPath := strings.Replace(fullPath, "models.pb.go", "service.pb.go", 1)
	if content, err := os.ReadFile(servicePbPath); err == nil {
		sb.WriteString("// ============================================\n")
		sb.WriteString("// Generated Go types from service.pb.go:\n")
		sb.WriteString("// ============================================\n\n")
		sb.WriteString(extractTypeDefinitions(string(content)))
		sb.WriteString("\n\n")
	}

	// Try alternate naming: *_grpc.pb.go or just look for any .pb.go in dir
	dir := filepath.Dir(fullPath)
	if files, err := os.ReadDir(dir); err == nil {
		for _, f := range files {
			if strings.HasSuffix(f.Name(), ".pb.go") {
				fPath := filepath.Join(dir, f.Name())
				// Skip if we already read it
				if fPath == fullPath || fPath == servicePbPath {
					continue
				}
				if content, err := os.ReadFile(fPath); err == nil {
					sb.WriteString(fmt.Sprintf("// ============================================\n"))
					sb.WriteString(fmt.Sprintf("// Generated Go types from %s:\n", f.Name()))
					sb.WriteString(fmt.Sprintf("// ============================================\n\n"))
					sb.WriteString(extractTypeDefinitions(string(content)))
					sb.WriteString("\n\n")
				}
			}
		}
	}

	return sb.String()
}

// extractTypeDefinitions pulls out type definitions from .pb.go content
// This gives Claude the exact struct definitions to work with
func extractTypeDefinitions(content string) string {
	var sb strings.Builder
	lines := strings.Split(content, "\n")
	inType := false
	braceCount := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Start of type definition
		if strings.HasPrefix(trimmed, "type ") && (strings.Contains(line, " struct {") || strings.Contains(line, " int32")) {
			inType = true
			braceCount = 0
		}

		// Start of const block (for enums)
		if strings.HasPrefix(trimmed, "const (") {
			inType = true
			braceCount = 1
			sb.WriteString(line + "\n")
			continue
		}

		// Enum type definition
		if strings.HasPrefix(trimmed, "type ") && strings.HasSuffix(trimmed, "int32") {
			sb.WriteString(line + "\n")
			continue
		}

		if inType {
			sb.WriteString(line + "\n")

			// Count braces
			braceCount += strings.Count(line, "{")
			braceCount -= strings.Count(line, "}")

			// End of type definition
			if braceCount <= 0 && (strings.HasPrefix(trimmed, "}") || !strings.Contains(line, "{")) {
				if strings.HasPrefix(trimmed, "}") {
					inType = false
					sb.WriteString("\n")
				}
			}
		}

		// Also capture method signatures that might be useful
		if strings.Contains(line, "func (x *") && strings.Contains(line, ") Get") {
			sb.WriteString(line + "\n")
		}
	}

	return sb.String()
}

// =============================================================================
// CLAUDE API
// =============================================================================

type ClaudeRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	Messages  []Message `json:"messages"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ClaudeResponse struct {
	Content []ContentBlock `json:"content"`
	Error   *ClaudeError   `json:"error,omitempty"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type ClaudeError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func CallClaude(apiKey string, prompt string) (string, error) {
	reqBody := ClaudeRequest{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 8192,
		Messages: []Message{
			{Role: "user", Content: prompt},
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var claudeResp ClaudeResponse
	if err := json.Unmarshal(body, &claudeResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if claudeResp.Error != nil {
		return "", fmt.Errorf("claude API error: %s", claudeResp.Error.Message)
	}

	if len(claudeResp.Content) == 0 {
		return "", fmt.Errorf("empty response from Claude")
	}

	return claudeResp.Content[0].Text, nil
}

// =============================================================================
// PROMPT BUILDING
// =============================================================================

func BuildPrompt(protoContext string, pbGoTypes string, directive LLMDirective, pkgName string) string {
	return fmt.Sprintf(`You are generating Go business logic for a gRPC/Connect service.

<proto_definitions>
%s
</proto_definitions>

<generated_go_types>
%s
</generated_go_types>

<task>
Generate the business logic for the %s.%s method.

Directive: %s

Input type: *%s
Output type: *%s
</task>

<requirements>
1. Generate a Go file with package name: %s
2. Create a Logic struct with Before%s and After%s methods
3. Before method: validation, authorization, pre-processing (can modify request)
4. After method: side effects (emails, notifications, cache updates)
5. Use the EXACT Go types from <generated_go_types> - pointer types, slice types, etc.
6. Use descriptive error messages with proper error handling
7. Include necessary imports
8. Add comments explaining the logic
9. Assume repositories are available via the Logic struct fields
10. Return ONLY the Go code, no markdown fences
11. Use the correct field access patterns from the generated types (e.g., req.GetUser(), not req.User)
</requirements>

<example_structure>
package %s

import (
	"context"
	"fmt"
	
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// %sLogic handles business logic for %s operations
type %sLogic struct {
	// Add repository dependencies here based on what the directive needs
}

// New%sLogic creates a new logic handler
func New%sLogic() *%sLogic {
	return &%sLogic{}
}

// Before%s runs before the %s operation
// Directive: %s
func (l *%sLogic) Before%s(ctx context.Context, req *%s) error {
	// Validation and pre-processing based on directive
	return nil
}

// After%s runs after the %s operation
func (l *%sLogic) After%s(ctx context.Context, req *%s, result *%s) error {
	// Side effects based on directive
	return nil
}
</example_structure>

Generate the complete implementation based on the directive. Use the exact types shown in <generated_go_types>.`,
		protoContext,
		pbGoTypes,
		directive.ServiceName, directive.MethodName,
		directive.Directive,
		directive.InputType,
		directive.OutputType,
		pkgName,
		directive.MethodName, directive.MethodName,
		pkgName,
		directive.MethodName, directive.MethodName,
		directive.MethodName,
		directive.MethodName,
		directive.MethodName, directive.MethodName,
		directive.MethodName,
		directive.Directive,
		directive.MethodName, directive.MethodName, directive.InputType,
		directive.MethodName, directive.MethodName,
		directive.MethodName, directive.MethodName, directive.InputType, directive.OutputType,
	)
}

// =============================================================================
// FALLBACK GENERATOR (when no API key)
// =============================================================================

func GenerateFallbackLogic(directive LLMDirective, pkgName string) Code {
	method := directive.MethodName
	input := directive.InputType
	output := directive.OutputType

	return Concat(CodeMonoid, []Code{
		Line("// Code generated by protoc-gen-llm. DO NOT EDIT."),
		Line("// NOTE: Generated without Claude API - implement TODO sections"),
		Linef("// Directive: %s", directive.Directive),
		Blank(),
		Linef("package %s", pkgName),
		Blank(),
		Line("import ("),
		Line(`	"context"`),
		Line(`	"fmt"`),
		Line(")"),
		Blank(),
		Linef("// %sLogic handles business logic for %s", method, method),
		Linef("type %sLogic struct {", method),
		Line("	// TODO: Add repository dependencies"),
		Line("}"),
		Blank(),
		Linef("// New%sLogic creates a new logic handler", method),
		Linef("func New%sLogic() *%sLogic {", method, method),
		Linef("	return &%sLogic{}", method),
		Line("}"),
		Blank(),
		Linef("// Before%s runs before the operation", method),
		Linef("// Directive: %s", directive.Directive),
		Linef("func (l *%sLogic) Before%s(ctx context.Context, req *%s) error {", method, method, input),
		Line("	// TODO: Implement based on directive:"),
		Linef("	// %s", directive.Directive),
		Line("	return nil"),
		Line("}"),
		Blank(),
		Linef("// After%s runs after the operation", method),
		Linef("func (l *%sLogic) After%s(ctx context.Context, req *%s, result *%s) error {", method, method, input, output),
		Line("	// TODO: Implement side effects based on directive"),
		Line("	return nil"),
		Line("}"),
		Blank(),
	})
}

// =============================================================================
// LOGIC INTERFACE GENERATOR
// =============================================================================

func GenerateLogicInterface(directives []LLMDirective, pkgName string) Code {
	if len(directives) == 0 {
		return CodeMonoid.Empty()
	}

	// Group by service
	serviceMap := make(map[string][]LLMDirective)
	for _, d := range directives {
		serviceMap[d.ServiceName] = append(serviceMap[d.ServiceName], d)
	}

	return Concat(CodeMonoid, []Code{
		Line("// Code generated by protoc-gen-llm. DO NOT EDIT."),
		Blank(),
		Linef("package %s", pkgName),
		Blank(),
		Line("import ("),
		Line(`	"context"`),
		Line(")"),
		Blank(),
		FoldMap(mapToSlice(serviceMap), CodeMonoid, func(kv kvPair) Code {
			return GenerateServiceLogicInterface(kv.key, kv.values)
		}),
	})
}

type kvPair struct {
	key    string
	values []LLMDirective
}

func mapToSlice(m map[string][]LLMDirective) []kvPair {
	var result []kvPair
	for k, v := range m {
		result = append(result, kvPair{k, v})
	}
	return result
}

func GenerateServiceLogicInterface(serviceName string, directives []LLMDirective) Code {
	return Concat(CodeMonoid, []Code{
		Linef("// %sLogic defines business logic hooks for %s", serviceName, serviceName),
		Linef("type %sLogic interface {", serviceName),
		FoldMap(directives, CodeMonoid, func(d LLMDirective) Code {
			return Concat(CodeMonoid, []Code{
				Linef("	Before%s(ctx context.Context, req *%s) error", d.MethodName, d.InputType),
				Linef("	After%s(ctx context.Context, req *%s, result *%s) error", d.MethodName, d.InputType, d.OutputType),
			})
		}),
		Line("}"),
		Blank(),
		Linef("// %sLogicNoop is a no-op implementation", serviceName),
		Linef("type %sLogicNoop struct{}", serviceName),
		Blank(),
		FoldMap(directives, CodeMonoid, func(d LLMDirective) Code {
			return Concat(CodeMonoid, []Code{
				Linef("func (l *%sLogicNoop) Before%s(ctx context.Context, req *%s) error { return nil }", serviceName, d.MethodName, d.InputType),
				Linef("func (l *%sLogicNoop) After%s(ctx context.Context, req *%s, result *%s) error { return nil }", serviceName, d.MethodName, d.InputType, d.OutputType),
			})
		}),
		Blank(),
	})
}

// =============================================================================
// MAIN
// =============================================================================

func main() {
	protogen.Options{}.Run(func(gen *protogen.Plugin) error {
		gen.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)
		apiKey := os.Getenv("ANTHROPIC_API_KEY")

		for _, f := range gen.Files {
			if !f.Generate {
				continue
			}

			protoContext := ExtractProtoContext(f)
			pbGoTypes := ReadGeneratedPbFiles(f)
			pkgName := string(f.GoPackageName)

			// =================================================================
			// BACKEND: Generate business logic for @llm directives
			// =================================================================
			var allDirectives []LLMDirective
			for _, svc := range f.Services {
				directives := ExtractLLMDirectives(svc)
				allDirectives = append(allDirectives, directives...)
			}

			if len(allDirectives) > 0 {
				// Generate interface file
				interfaceFile := gen.NewGeneratedFile(f.GeneratedFilenamePrefix+"_logic_interface.pb.go", f.GoImportPath)
				interfaceFile.P(GenerateLogicInterface(allDirectives, pkgName).Run())

				// Generate logic for each directive
				for _, directive := range allDirectives {
					fileName := fmt.Sprintf("%s_%s_logic.pb.go",
						f.GeneratedFilenamePrefix,
						toSnakeCase(directive.MethodName))

					logicFile := gen.NewGeneratedFile(fileName, f.GoImportPath)

					if apiKey == "" {
						logicFile.P(GenerateFallbackLogic(directive, pkgName).Run())
					} else {
						prompt := BuildPrompt(protoContext, pbGoTypes, directive, pkgName)
						response, err := CallClaude(apiKey, prompt)
						if err != nil {
							logicFile.P(fmt.Sprintf("// Claude API error: %s\n\n", err))
							logicFile.P(GenerateFallbackLogic(directive, pkgName).Run())
						} else {
							response = cleanCodeResponse(response)
							logicFile.P(response)
						}
					}
				}
			}

			// =================================================================
			// FRONTEND: Generate React UI pages (only with API key)
			// =================================================================
			if apiKey != "" && len(f.Messages) > 0 {
				// Extract entities (messages with ID field)
				var entities []EntityInfo
				for _, msg := range f.Messages {
					if hasIDField(msg) {
						entities = append(entities, ExtractEntityInfo(msg))
					}
				}

				if len(entities) > 0 {
					// Generate UI pages for each entity
					for _, entity := range entities {
						generateUIPages(gen, f, entity, protoContext, apiKey)
					}
				}
			}
		}
		return nil
	})
}

// =============================================================================
// ENTITY INFO FOR UI GENERATION
// =============================================================================

type EntityInfo struct {
	Name   string
	Fields []EntityField
}

type EntityField struct {
	Name       string
	ProtoType  string
	GoType     string
	IsEnum     bool
	EnumValues []string
}

func hasIDField(msg *protogen.Message) bool {
	for _, field := range msg.Fields {
		if strings.EqualFold(string(field.Desc.Name()), "id") {
			return true
		}
	}
	return false
}

func ExtractEntityInfo(msg *protogen.Message) EntityInfo {
	var fields []EntityField
	for _, field := range msg.Fields {
		ef := EntityField{
			Name:      string(field.Desc.Name()),
			ProtoType: field.Desc.Kind().String(),
			GoType:    fieldGoType(field),
			IsEnum:    field.Desc.Kind().String() == "enum",
		}
		if ef.IsEnum && field.Enum != nil {
			for _, v := range field.Enum.Values {
				ef.EnumValues = append(ef.EnumValues, string(v.Desc.Name()))
			}
		}
		fields = append(fields, ef)
	}
	return EntityInfo{
		Name:   msg.GoIdent.GoName,
		Fields: fields,
	}
}

func fieldGoType(field *protogen.Field) string {
	switch field.Desc.Kind().String() {
	case "string":
		return "string"
	case "int32", "int64", "uint32", "uint64":
		return "number"
	case "bool":
		return "boolean"
	case "message":
		if field.Message != nil {
			return field.Message.GoIdent.GoName
		}
		return "object"
	case "enum":
		if field.Enum != nil {
			return field.Enum.GoIdent.GoName
		}
		return "enum"
	default:
		return "any"
	}
}

// =============================================================================
// UI PAGE GENERATION
// =============================================================================

func generateUIPages(gen *protogen.Plugin, f *protogen.File, entity EntityInfo, protoContext string, apiKey string) {
	basePath := strings.Replace(f.GeneratedFilenamePrefix, "/go/", "/ui/pages/", 1)

	pageTypes := []string{"List", "Detail", "Create", "Edit"}

	for _, pageType := range pageTypes {
		fileName := fmt.Sprintf("%s_%s_%s_page.tsx",
			basePath,
			toSnakeCase(entity.Name),
			strings.ToLower(pageType))

		pageFile := gen.NewGeneratedFile(fileName, "")

		prompt := BuildUIPrompt(protoContext, entity, pageType)
		response, err := CallClaude(apiKey, prompt)
		if err != nil {
			pageFile.P(fmt.Sprintf("// Claude API error: %s\n", err))
			pageFile.P(GenerateFallbackUIPage(entity, pageType).Run())
		} else {
			response = cleanTsxResponse(response)
			pageFile.P(response)
		}
	}

	// Also generate Form and Table components
	componentTypes := []string{"Form", "Table"}
	componentBasePath := strings.Replace(f.GeneratedFilenamePrefix, "/go/", "/ui/components/", 1)

	for _, compType := range componentTypes {
		fileName := fmt.Sprintf("%s_%s_%s.tsx",
			componentBasePath,
			toSnakeCase(entity.Name),
			strings.ToLower(compType))

		compFile := gen.NewGeneratedFile(fileName, "")

		prompt := BuildUIComponentPrompt(protoContext, entity, compType)
		response, err := CallClaude(apiKey, prompt)
		if err != nil {
			compFile.P(fmt.Sprintf("// Claude API error: %s\n", err))
			compFile.P(GenerateFallbackUIComponent(entity, compType).Run())
		} else {
			response = cleanTsxResponse(response)
			compFile.P(response)
		}
	}
}

func BuildUIPrompt(protoContext string, entity EntityInfo, pageType string) string {
	fieldsDesc := describeFields(entity.Fields)

	return fmt.Sprintf(`You are generating a React TypeScript page component for an admin UI.

<proto_definitions>
%s
</proto_definitions>

<entity>
Name: %s
Fields:
%s
</entity>

<task>
Generate a %sPage component for the %s entity.
</task>

<requirements>
1. Use React functional component with TypeScript
2. Use react-router-dom for navigation (useNavigate, useParams, Link)
3. Use these custom hooks from "../hooks":
   - use%ss() - list all
   - use%s(id) - get one by id
   - useCreate%s() - create mutation
   - useUpdate%s() - update mutation
   - useDelete%s() - delete mutation
4. Use Tailwind CSS for styling
5. Handle loading, error, and empty states
6. Make smart UI decisions based on field names and types:
   - email fields → type="email"
   - password fields → type="password"
   - url/website fields → type="url"
   - description/bio/content → textarea
   - count/quantity/amount → type="number"
   - status/type/role (enums) → select dropdown
   - created_at/updated_at → display as formatted date, readonly
   - id → hidden or readonly
7. Return ONLY the TSX code, no markdown fences
8. Export the component as named export
</requirements>`,
		protoContext,
		entity.Name,
		fieldsDesc,
		pageType, entity.Name,
		entity.Name,
		entity.Name,
		entity.Name,
		entity.Name,
		entity.Name,
	)
}

func BuildUIComponentPrompt(protoContext string, entity EntityInfo, compType string) string {
	fieldsDesc := describeFields(entity.Fields)

	if compType == "Form" {
		return fmt.Sprintf(`You are generating a React TypeScript form component.

<proto_definitions>
%s
</proto_definitions>

<entity>
Name: %s
Fields:
%s
</entity>

<task>
Generate a %sForm component that can be used for both Create and Edit.
</task>

<requirements>
1. Use React functional component with TypeScript
2. Props: initial?: %s, onSubmit: (data) => void, onCancel: () => void, isLoading?: boolean
3. Use useState for form state
4. Use Tailwind CSS for styling
5. Make smart input decisions based on field names and types:
   - email → type="email" with validation
   - password → type="password"
   - url/website → type="url"
   - description/bio/content → textarea
   - count/quantity/amount → type="number" with min=0
   - enums → select dropdown with options
   - created_at/updated_at/id → don't include in form (readonly)
   - phone → type="tel"
   - boolean → checkbox
6. Add proper labels and placeholder text
7. Show validation errors
8. Return ONLY the TSX code, no markdown fences
9. Export as named export
</requirements>`,
			protoContext,
			entity.Name,
			fieldsDesc,
			entity.Name,
			entity.Name,
		)
	}

	// Table component
	return fmt.Sprintf(`You are generating a React TypeScript table component.

<proto_definitions>
%s
</proto_definitions>

<entity>
Name: %s
Fields:
%s
</entity>

<task>
Generate a %sTable component for displaying a list.
</task>

<requirements>
1. Use React functional component with TypeScript
2. Props: data: %s[], onEdit?: (id: string) => void, onDelete?: (id: string) => void
3. Use Tailwind CSS for styling
4. Smart column display:
   - Skip long text fields (description, content, bio)
   - Format dates nicely
   - Show enum values as badges
   - Truncate long strings
   - Show boolean as Yes/No or checkmark
5. Include action buttons (View, Edit, Delete)
6. Handle empty state
7. Return ONLY the TSX code, no markdown fences
8. Export as named export
</requirements>`,
		protoContext,
		entity.Name,
		fieldsDesc,
		entity.Name,
		entity.Name,
	)
}

func describeFields(fields []EntityField) string {
	var lines []string
	for _, f := range fields {
		enumInfo := ""
		if f.IsEnum && len(f.EnumValues) > 0 {
			enumInfo = fmt.Sprintf(" (enum: %s)", strings.Join(f.EnumValues, ", "))
		}
		lines = append(lines, fmt.Sprintf("  - %s: %s%s", f.Name, f.ProtoType, enumInfo))
	}
	return strings.Join(lines, "\n")
}

func GenerateFallbackUIPage(entity EntityInfo, pageType string) Code {
	return Concat(CodeMonoid, []Code{
		Line("// TODO: Implement with Claude API"),
		Linef("// Entity: %s, Page: %s", entity.Name, pageType),
		Blank(),
		Line("import React from 'react';"),
		Blank(),
		Linef("export const %s%sPage: React.FC = () => {", entity.Name, pageType),
		Linef("  return <div>%s %s Page - TODO</div>;", entity.Name, pageType),
		Line("};"),
	})
}

func GenerateFallbackUIComponent(entity EntityInfo, compType string) Code {
	return Concat(CodeMonoid, []Code{
		Line("// TODO: Implement with Claude API"),
		Linef("// Entity: %s, Component: %s", entity.Name, compType),
		Blank(),
		Line("import React from 'react';"),
		Blank(),
		Linef("export const %s%s: React.FC = () => {", entity.Name, compType),
		Linef("  return <div>%s %s - TODO</div>;", entity.Name, compType),
		Line("};"),
	})
}

func cleanTsxResponse(s string) string {
	s = strings.TrimPrefix(s, "```tsx\n")
	s = strings.TrimPrefix(s, "```typescript\n")
	s = strings.TrimPrefix(s, "```\n")
	s = strings.TrimSuffix(s, "\n```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

func cleanCodeResponse(s string) string {
	s = strings.TrimPrefix(s, "```go\n")
	s = strings.TrimPrefix(s, "```\n")
	s = strings.TrimSuffix(s, "\n```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
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

func lowerFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}
