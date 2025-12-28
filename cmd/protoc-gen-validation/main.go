// protoc-gen-validation generates validation logic for Go + TypeScript
// Uses Category Theory: Monoid + Functor + Fold
// Infers rules from field names/types, or uses proto options
package main

import (
	"fmt"
	"strings"
	"unicode"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoreflect"
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

func Line(s string) Code                            { return Code{Run: func() string { return s + "\n" }} }
func Linef(format string, args ...interface{}) Code { return Line(fmt.Sprintf(format, args...)) }
func Blank() Code                                   { return Line("") }

// =============================================================================
// FIELD INFO WITH VALIDATION RULES
// =============================================================================

type MessageInfo struct {
	Name, GoName string
	Fields       []FieldInfo
}

type FieldInfo struct {
	Name, GoName, GoType, TsType string
	Rules                        []ValidationRule
}

type ValidationRule struct {
	Name    string // required, email, min_len, max_len, min, max, pattern, uuid
	Param   string // parameter value if applicable
	Message string // error message
}

func ExtractMessageInfo(msg *protogen.Message) MessageInfo {
	return MessageInfo{
		Name:   string(msg.Desc.Name()),
		GoName: msg.GoIdent.GoName,
		Fields: Map(msg.Fields, ExtractFieldInfo),
	}
}

func ExtractFieldInfo(field *protogen.Field) FieldInfo {
	name := string(field.Desc.Name())
	goType, tsType := fieldTypes(field)
	rules := inferValidationRules(name, field)

	return FieldInfo{
		Name:   name,
		GoName: field.GoName,
		GoType: goType,
		TsType: tsType,
		Rules:  rules,
	}
}

func fieldTypes(field *protogen.Field) (string, string) {
	switch field.Desc.Kind() {
	case protoreflect.BoolKind:
		return "bool", "boolean"
	case protoreflect.Int32Kind, protoreflect.Sint32Kind:
		return "int32", "number"
	case protoreflect.Int64Kind, protoreflect.Sint64Kind:
		return "int64", "number"
	case protoreflect.Uint32Kind:
		return "uint32", "number"
	case protoreflect.Uint64Kind:
		return "uint64", "number"
	case protoreflect.FloatKind:
		return "float32", "number"
	case protoreflect.DoubleKind:
		return "float64", "number"
	case protoreflect.StringKind:
		return "string", "string"
	case protoreflect.EnumKind:
		return field.Enum.GoIdent.GoName, field.Enum.GoIdent.GoName
	case protoreflect.MessageKind:
		return "*" + field.Message.GoIdent.GoName, field.Message.GoIdent.GoName
	default:
		return "interface{}", "unknown"
	}
}

// Infer validation rules from field name and type
func inferValidationRules(name string, field *protogen.Field) []ValidationRule {
	var rules []ValidationRule
	lower := strings.ToLower(name)

	// Required fields (inferred from common patterns)
	if lower == "email" || lower == "name" || lower == "title" {
		rules = append(rules, ValidationRule{Name: "required", Message: fmt.Sprintf("%s is required", name)})
	}

	// String validations
	if field.Desc.Kind() == protoreflect.StringKind {
		// Email
		if strings.Contains(lower, "email") {
			rules = append(rules, ValidationRule{Name: "email", Message: "must be a valid email address"})
		}

		// URL
		if strings.Contains(lower, "url") || strings.Contains(lower, "link") || strings.Contains(lower, "website") {
			rules = append(rules, ValidationRule{Name: "url", Message: "must be a valid URL"})
		}

		// UUID
		if lower == "id" || strings.HasSuffix(lower, "_id") {
			rules = append(rules, ValidationRule{Name: "uuid", Message: "must be a valid UUID"})
		}

		// Phone
		if strings.Contains(lower, "phone") {
			rules = append(rules, ValidationRule{Name: "phone", Message: "must be a valid phone number"})
		}

		// Password
		if strings.Contains(lower, "password") {
			rules = append(rules, ValidationRule{Name: "min_len", Param: "8", Message: "must be at least 8 characters"})
		}

		// Slug
		if strings.Contains(lower, "slug") {
			rules = append(rules, ValidationRule{Name: "slug", Message: "must contain only lowercase letters, numbers, and hyphens"})
		}

		// Username
		if strings.Contains(lower, "username") {
			rules = append(rules, ValidationRule{Name: "min_len", Param: "3", Message: "must be at least 3 characters"})
			rules = append(rules, ValidationRule{Name: "max_len", Param: "30", Message: "must be at most 30 characters"})
			rules = append(rules, ValidationRule{Name: "alphanum", Message: "must contain only letters and numbers"})
		}

		// Name fields - reasonable length
		if lower == "name" || strings.HasSuffix(lower, "_name") || lower == "title" {
			rules = append(rules, ValidationRule{Name: "max_len", Param: "255", Message: "must be at most 255 characters"})
		}

		// Description/content - longer limit
		if strings.Contains(lower, "description") || strings.Contains(lower, "content") || strings.Contains(lower, "bio") {
			rules = append(rules, ValidationRule{Name: "max_len", Param: "10000", Message: "must be at most 10000 characters"})
		}
	}

	// Numeric validations
	if field.Desc.Kind() == protoreflect.Int32Kind || field.Desc.Kind() == protoreflect.Int64Kind {
		// Age
		if lower == "age" {
			rules = append(rules, ValidationRule{Name: "min", Param: "0", Message: "must be at least 0"})
			rules = append(rules, ValidationRule{Name: "max", Param: "150", Message: "must be at most 150"})
		}

		// Count/quantity
		if strings.Contains(lower, "count") || strings.Contains(lower, "quantity") || strings.Contains(lower, "amount") {
			rules = append(rules, ValidationRule{Name: "min", Param: "0", Message: "must be non-negative"})
		}

		// Price (in cents)
		if strings.Contains(lower, "price") || strings.Contains(lower, "cost") {
			rules = append(rules, ValidationRule{Name: "min", Param: "0", Message: "must be non-negative"})
		}

		// Page size
		if lower == "page_size" || lower == "limit" {
			rules = append(rules, ValidationRule{Name: "min", Param: "1", Message: "must be at least 1"})
			rules = append(rules, ValidationRule{Name: "max", Param: "1000", Message: "must be at most 1000"})
		}
	}

	// Enum - must be valid value (non-zero usually)
	if field.Desc.Kind() == protoreflect.EnumKind {
		// Skip UNSPECIFIED (usually value 0)
		rules = append(rules, ValidationRule{Name: "enum", Message: "must be a valid value"})
	}

	return rules
}

// =============================================================================
// GO VALIDATION GENERATOR
// =============================================================================

func GenerateGoValidation(messages []MessageInfo, pkgName string) Code {
	return Concat(CodeMonoid, []Code{
		Line("// Code generated by protoc-gen-validation. DO NOT EDIT."),
		Blank(),
		Linef("package %s", pkgName),
		Blank(),
		Line("import ("),
		Line(`	"errors"`),
		Line(`	"fmt"`),
		Line(`	"net/mail"`),
		Line(`	"net/url"`),
		Line(`	"regexp"`),
		Line(`	"strings"`),
		Line(`	"unicode"`),
		Line(")"),
		Blank(),
		Line("// ValidationError contains field-level validation errors"),
		Line("type ValidationError struct {"),
		Line("	Field   string"),
		Line("	Message string"),
		Line("}"),
		Blank(),
		Line("func (e ValidationError) Error() string {"),
		Line(`	return fmt.Sprintf("%s: %s", e.Field, e.Message)`),
		Line("}"),
		Blank(),
		Line("// ValidationErrors is a collection of validation errors"),
		Line("type ValidationErrors []ValidationError"),
		Blank(),
		Line("func (e ValidationErrors) Error() string {"),
		Line("	if len(e) == 0 {"),
		Line(`		return ""`),
		Line("	}"),
		Line("	var msgs []string"),
		Line("	for _, err := range e {"),
		Line("		msgs = append(msgs, err.Error())"),
		Line("	}"),
		Line(`	return strings.Join(msgs, "; ")`),
		Line("}"),
		Blank(),
		Line("func (e ValidationErrors) HasErrors() bool {"),
		Line("	return len(e) > 0"),
		Line("}"),
		Blank(),
		Line("// Helper validators"),
		Line("var ("),
		Line(`	uuidRegex     = regexp.MustCompile("^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$")`),
		Line(`	slugRegex     = regexp.MustCompile("^[a-z0-9]+(?:-[a-z0-9]+)*$")`),
		Line(`	alphanumRegex = regexp.MustCompile("^[a-zA-Z0-9]+$")`),
		Line(`	phoneRegex    = regexp.MustCompile("^[+]?[0-9\\-\\s()]{7,20}$")`),
		Line(")"),
		Blank(),
		FoldMap(messages, CodeMonoid, GenerateGoValidator),
	})
}

func GenerateGoValidator(m MessageInfo) Code {
	fieldsWithRules := Filter(m.Fields, func(f FieldInfo) bool { return len(f.Rules) > 0 })
	if len(fieldsWithRules) == 0 {
		return CodeMonoid.Empty()
	}

	return Concat(CodeMonoid, []Code{
		Linef("// Validate%s validates a %s", m.GoName, m.GoName),
		Linef("func Validate%s(m *%s) ValidationErrors {", m.GoName, m.GoName),
		Line("	var errs ValidationErrors"),
		Line("	if m == nil {"),
		Line(`		return append(errs, ValidationError{Field: "_", Message: "cannot be nil"})`),
		Line("	}"),
		Blank(),
		FoldMap(fieldsWithRules, CodeMonoid, GenerateGoFieldValidation),
		Line("	return errs"),
		Line("}"),
		Blank(),
		Linef("// MustValidate%s panics if validation fails", m.GoName),
		Linef("func MustValidate%s(m *%s) {", m.GoName, m.GoName),
		Linef("	if errs := Validate%s(m); errs.HasErrors() {", m.GoName),
		Line("		panic(errs)"),
		Line("	}"),
		Line("}"),
		Blank(),
	})
}

func GenerateGoFieldValidation(f FieldInfo) Code {
	fieldAccess := "m." + f.GoName
	fieldName := toSnakeCase(f.Name)

	validations := FoldMap(f.Rules, CodeMonoid, func(r ValidationRule) Code {
		switch r.Name {
		case "required":
			if f.GoType == "string" {
				return Concat(CodeMonoid, []Code{
					Linef("	if %s == \"\" {", fieldAccess),
					Linef("		errs = append(errs, ValidationError{Field: \"%s\", Message: \"%s\"})", fieldName, r.Message),
					Line("	}"),
				})
			}
		case "email":
			return Concat(CodeMonoid, []Code{
				Linef("	if %s != \"\" {", fieldAccess),
				Linef("		if _, err := mail.ParseAddress(%s); err != nil {", fieldAccess),
				Linef("			errs = append(errs, ValidationError{Field: \"%s\", Message: \"%s\"})", fieldName, r.Message),
				Line("		}"),
				Line("	}"),
			})
		case "url":
			return Concat(CodeMonoid, []Code{
				Linef("	if %s != \"\" {", fieldAccess),
				Linef("		if _, err := url.ParseRequestURI(%s); err != nil {", fieldAccess),
				Linef("			errs = append(errs, ValidationError{Field: \"%s\", Message: \"%s\"})", fieldName, r.Message),
				Line("		}"),
				Line("	}"),
			})
		case "uuid":
			return Concat(CodeMonoid, []Code{
				Linef("	if %s != \"\" && !uuidRegex.MatchString(%s) {", fieldAccess, fieldAccess),
				Linef("		errs = append(errs, ValidationError{Field: \"%s\", Message: \"%s\"})", fieldName, r.Message),
				Line("	}"),
			})
		case "slug":
			return Concat(CodeMonoid, []Code{
				Linef("	if %s != \"\" && !slugRegex.MatchString(%s) {", fieldAccess, fieldAccess),
				Linef("		errs = append(errs, ValidationError{Field: \"%s\", Message: \"%s\"})", fieldName, r.Message),
				Line("	}"),
			})
		case "alphanum":
			return Concat(CodeMonoid, []Code{
				Linef("	if %s != \"\" && !alphanumRegex.MatchString(%s) {", fieldAccess, fieldAccess),
				Linef("		errs = append(errs, ValidationError{Field: \"%s\", Message: \"%s\"})", fieldName, r.Message),
				Line("	}"),
			})
		case "phone":
			return Concat(CodeMonoid, []Code{
				Linef("	if %s != \"\" && !phoneRegex.MatchString(%s) {", fieldAccess, fieldAccess),
				Linef("		errs = append(errs, ValidationError{Field: \"%s\", Message: \"%s\"})", fieldName, r.Message),
				Line("	}"),
			})
		case "min_len":
			return Concat(CodeMonoid, []Code{
				Linef("	if len(%s) > 0 && len(%s) < %s {", fieldAccess, fieldAccess, r.Param),
				Linef("		errs = append(errs, ValidationError{Field: \"%s\", Message: \"%s\"})", fieldName, r.Message),
				Line("	}"),
			})
		case "max_len":
			return Concat(CodeMonoid, []Code{
				Linef("	if len(%s) > %s {", fieldAccess, r.Param),
				Linef("		errs = append(errs, ValidationError{Field: \"%s\", Message: \"%s\"})", fieldName, r.Message),
				Line("	}"),
			})
		case "min":
			return Concat(CodeMonoid, []Code{
				Linef("	if %s < %s {", fieldAccess, r.Param),
				Linef("		errs = append(errs, ValidationError{Field: \"%s\", Message: \"%s\"})", fieldName, r.Message),
				Line("	}"),
			})
		case "max":
			return Concat(CodeMonoid, []Code{
				Linef("	if %s > %s {", fieldAccess, r.Param),
				Linef("		errs = append(errs, ValidationError{Field: \"%s\", Message: \"%s\"})", fieldName, r.Message),
				Line("	}"),
			})
		case "enum":
			return Concat(CodeMonoid, []Code{
				Linef("	if %s == 0 {", fieldAccess),
				Linef("		errs = append(errs, ValidationError{Field: \"%s\", Message: \"%s\"})", fieldName, r.Message),
				Line("	}"),
			})
		}
		return CodeMonoid.Empty()
	})

	return validations
}

// =============================================================================
// TYPESCRIPT VALIDATION GENERATOR
// =============================================================================

func GenerateTsValidation(messages []MessageInfo) Code {
	return Concat(CodeMonoid, []Code{
		Line("// Code generated by protoc-gen-validation. DO NOT EDIT."),
		Blank(),
		Line("export interface ValidationError {"),
		Line("  field: string;"),
		Line("  message: string;"),
		Line("}"),
		Blank(),
		Line("export interface ValidationResult {"),
		Line("  valid: boolean;"),
		Line("  errors: Record<string, string>;"),
		Line("}"),
		Blank(),
		Line("// Helper validators"),
		Line("const validators = {"),
		Line("  email: (v: string) => /^[^\\s@]+@[^\\s@]+\\.[^\\s@]+$/.test(v),"),
		Line("  url: (v: string) => { try { new URL(v); return true; } catch { return false; } },"),
		Line("  uuid: (v: string) => /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/.test(v),"),
		Line("  slug: (v: string) => /^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(v),"),
		Line("  alphanum: (v: string) => /^[a-zA-Z0-9]+$/.test(v),"),
		Line("  phone: (v: string) => /^[+]?[0-9\\-\\s()]{7,20}$/.test(v),"),
		Line("};"),
		Blank(),
		FoldMap(messages, CodeMonoid, GenerateTsValidator),
	})
}

func GenerateTsValidator(m MessageInfo) Code {
	fieldsWithRules := Filter(m.Fields, func(f FieldInfo) bool { return len(f.Rules) > 0 })
	if len(fieldsWithRules) == 0 {
		return CodeMonoid.Empty()
	}

	return Concat(CodeMonoid, []Code{
		Linef("export function validate%s(data: Partial<%s>): ValidationResult {", m.GoName, m.GoName),
		Line("  const errors: Record<string, string> = {};"),
		Blank(),
		FoldMap(fieldsWithRules, CodeMonoid, GenerateTsFieldValidation),
		Blank(),
		Line("  return { valid: Object.keys(errors).length === 0, errors };"),
		Line("}"),
		Blank(),
		Linef("export function use%sValidation() {", m.GoName),
		Line("  const [errors, setErrors] = useState<Record<string, string>>({});"),
		Blank(),
		Linef("  const validate = (data: Partial<%s>) => {", m.GoName),
		Linef("    const result = validate%s(data);", m.GoName),
		Line("    setErrors(result.errors);"),
		Line("    return result.valid;"),
		Line("  };"),
		Blank(),
		Line("  const clearErrors = () => setErrors({});"),
		Blank(),
		Line("  return { errors, validate, clearErrors };"),
		Line("}"),
		Blank(),
	})
}

func GenerateTsFieldValidation(f FieldInfo) Code {
	fieldName := lowerFirst(f.GoName)
	snakeName := toSnakeCase(f.Name)

	validations := FoldMap(f.Rules, CodeMonoid, func(r ValidationRule) Code {
		switch r.Name {
		case "required":
			return Concat(CodeMonoid, []Code{
				Linef("  if (!data.%s) {", fieldName),
				Linef("    errors.%s = \"%s\";", snakeName, r.Message),
				Line("  }"),
			})
		case "email":
			return Concat(CodeMonoid, []Code{
				Linef("  if (data.%s && !validators.email(data.%s)) {", fieldName, fieldName),
				Linef("    errors.%s = \"%s\";", snakeName, r.Message),
				Line("  }"),
			})
		case "url":
			return Concat(CodeMonoid, []Code{
				Linef("  if (data.%s && !validators.url(data.%s)) {", fieldName, fieldName),
				Linef("    errors.%s = \"%s\";", snakeName, r.Message),
				Line("  }"),
			})
		case "uuid":
			return Concat(CodeMonoid, []Code{
				Linef("  if (data.%s && !validators.uuid(data.%s)) {", fieldName, fieldName),
				Linef("    errors.%s = \"%s\";", snakeName, r.Message),
				Line("  }"),
			})
		case "slug":
			return Concat(CodeMonoid, []Code{
				Linef("  if (data.%s && !validators.slug(data.%s)) {", fieldName, fieldName),
				Linef("    errors.%s = \"%s\";", snakeName, r.Message),
				Line("  }"),
			})
		case "alphanum":
			return Concat(CodeMonoid, []Code{
				Linef("  if (data.%s && !validators.alphanum(data.%s)) {", fieldName, fieldName),
				Linef("    errors.%s = \"%s\";", snakeName, r.Message),
				Line("  }"),
			})
		case "phone":
			return Concat(CodeMonoid, []Code{
				Linef("  if (data.%s && !validators.phone(data.%s)) {", fieldName, fieldName),
				Linef("    errors.%s = \"%s\";", snakeName, r.Message),
				Line("  }"),
			})
		case "min_len":
			return Concat(CodeMonoid, []Code{
				Linef("  if (data.%s && data.%s.length < %s) {", fieldName, fieldName, r.Param),
				Linef("    errors.%s = \"%s\";", snakeName, r.Message),
				Line("  }"),
			})
		case "max_len":
			return Concat(CodeMonoid, []Code{
				Linef("  if (data.%s && data.%s.length > %s) {", fieldName, fieldName, r.Param),
				Linef("    errors.%s = \"%s\";", snakeName, r.Message),
				Line("  }"),
			})
		case "min":
			return Concat(CodeMonoid, []Code{
				Linef("  if (data.%s !== undefined && data.%s < %s) {", fieldName, fieldName, r.Param),
				Linef("    errors.%s = \"%s\";", snakeName, r.Message),
				Line("  }"),
			})
		case "max":
			return Concat(CodeMonoid, []Code{
				Linef("  if (data.%s !== undefined && data.%s > %s) {", fieldName, fieldName, r.Param),
				Linef("    errors.%s = \"%s\";", snakeName, r.Message),
				Line("  }"),
			})
		case "enum":
			return Concat(CodeMonoid, []Code{
				Linef("  if (!data.%s || data.%s === 0) {", fieldName, fieldName),
				Linef("    errors.%s = \"%s\";", snakeName, r.Message),
				Line("  }"),
			})
		}
		return CodeMonoid.Empty()
	})

	return validations
}

// =============================================================================
// MAIN
// =============================================================================

func main() {
	protogen.Options{}.Run(func(gen *protogen.Plugin) error {
		for _, f := range gen.Files {
			if !f.Generate || len(f.Messages) == 0 {
				continue
			}

			messages := Map(f.Messages, ExtractMessageInfo)
			pkgName := string(f.GoPackageName)

			// Generate Go validation
			goFile := gen.NewGeneratedFile(f.GeneratedFilenamePrefix+"_validation.pb.go", f.GoImportPath)
			goFile.P(GenerateGoValidation(messages, pkgName).Run())

			// Generate TypeScript validation
			tsFile := gen.NewGeneratedFile(f.GeneratedFilenamePrefix+"_validation.ts", f.GoImportPath)
			tsFile.P(GenerateTsValidation(messages).Run())
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
