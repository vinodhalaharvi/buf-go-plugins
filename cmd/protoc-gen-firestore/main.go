// protoc-gen-firestore generates Firestore CRUD stubs using Category Theory composition
// No string append - uses: Monoid, Functor (Map), Fold, When
//
// Usage: Add to your proto:
//
//	import "options/v1/options.proto";
//
//	message User {
//	  option (bufplugins.options.v1.entity) = {
//	    collection: "users"
//	    id_field: "user_id"
//	  };
//	  string user_id = 1;
//	  string email = 2;
//	}
package main

import (
	"fmt"
	"strings"
	"unicode"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	pluginpb "google.golang.org/protobuf/types/pluginpb"
)

// Extension field number for entity option (matches options.proto)
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

func Lit(s string) Code                                { return Code{Run: func() string { return s }} }
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

// =============================================================================
// GO CODE COMBINATORS
// =============================================================================

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

func VarBlock(vars Code) Code {
	return Concat(CodeMonoid, []Code{Line("var ("), Indent(vars), Line(")")})
}

func If(cond string, body Code) Code {
	return Concat(CodeMonoid, []Code{Linef("if %s {", cond), Indent(body), Line("}")})
}

func IfElse(cond string, ifBody, elseBody Code) Code {
	return Concat(CodeMonoid, []Code{Linef("if %s {", cond), Indent(ifBody), Line("} else {"), Indent(elseBody), Line("}")})
}

func Return(values ...string) Code {
	if len(values) == 0 {
		return Line("return")
	}
	return Linef("return %s", strings.Join(values, ", "))
}

// =============================================================================
// ENTITY OPTIONS (from proto options)
// =============================================================================

type EntityConfig struct {
	Generate   bool
	Collection string
	IDField    string
}

// getEntityConfig extracts entity options from message descriptor
func getEntityConfig(msg *protogen.Message) *EntityConfig {
	opts := msg.Desc.Options()
	if opts == nil {
		return nil
	}

	// Get the raw options to check for our extension
	optsProto, ok := opts.(*descriptorpb.MessageOptions)
	if !ok {
		return nil
	}

	// Check if our extension is present using proto reflection
	// Extension number 50000 is our entity option
	b, err := proto.Marshal(optsProto)
	if err != nil {
		return nil
	}

	// Look for extension field 50000 in the wire format
	// This is a simple check - if the message has unknown fields with our extension number
	if !hasExtension(b, entityExtensionNumber) {
		return nil
	}

	// Extension is present - extract values from unknown fields
	config := &EntityConfig{
		Generate:   true,
		Collection: toSnakeCase(string(msg.Desc.Name())) + "s",
		IDField:    findIDField(msg),
	}

	// Try to parse the extension data for custom values
	parseEntityExtension(optsProto, config)

	return config
}

// hasExtension checks if wire-format bytes contain an extension with given field number
func hasExtension(b []byte, fieldNum int32) bool {
	// Simple heuristic: check if ProtoReflect has unknown fields
	// For a more robust solution, we'd parse the wire format properly
	// But for our use case, just having the option present is enough
	return len(b) > 0 && containsFieldTag(b, fieldNum)
}

// containsFieldTag is a simple check for field presence in wire format
func containsFieldTag(b []byte, fieldNum int32) bool {
	// Wire format: (field_number << 3) | wire_type
	// For embedded message (wire type 2): tag = (fieldNum << 3) | 2
	expectedTag := uint64(fieldNum<<3 | 2)

	i := 0
	for i < len(b) {
		tag, n := decodeVarint(b[i:])
		if n == 0 {
			break
		}
		if tag == expectedTag {
			return true
		}
		i += n

		// Skip the value based on wire type
		wireType := tag & 0x7
		switch wireType {
		case 0: // Varint
			_, vn := decodeVarint(b[i:])
			i += vn
		case 1: // 64-bit
			i += 8
		case 2: // Length-delimited
			length, ln := decodeVarint(b[i:])
			i += ln + int(length)
		case 5: // 32-bit
			i += 4
		default:
			return false
		}
	}
	return false
}

func decodeVarint(b []byte) (uint64, int) {
	var x uint64
	var n int
	for n < len(b) && n < 10 {
		v := b[n]
		x |= uint64(v&0x7f) << (7 * n)
		n++
		if v < 0x80 {
			return x, n
		}
	}
	return 0, 0
}

// parseEntityExtension tries to extract custom values from the extension
func parseEntityExtension(opts *descriptorpb.MessageOptions, config *EntityConfig) {
	// This would require the generated options package to fully parse
	// For now, we rely on defaults and the proto-level extraction
	// The extension presence is enough to trigger generation
}

// findIDField finds the ID field in a message
func findIDField(msg *protogen.Message) string {
	// First look for "id" field
	for _, field := range msg.Fields {
		name := string(field.Desc.Name())
		if strings.EqualFold(name, "id") {
			return name
		}
	}
	// Then look for first field ending in "_id"
	for _, field := range msg.Fields {
		name := string(field.Desc.Name())
		if strings.HasSuffix(strings.ToLower(name), "_id") {
			return name
		}
	}
	// Default to first string field
	for _, field := range msg.Fields {
		if field.Desc.Kind() == protoreflect.StringKind {
			return string(field.Desc.Name())
		}
	}
	return "id"
}

// =============================================================================
// MESSAGE INFO (Pure data extraction)
// =============================================================================

type MessageInfo struct {
	Name, GoName, Collection, IDField, IDGoName     string
	Fields                                          []FieldInfo
	HasID, HasCreatedAt, HasUpdatedAt, HasDeletedAt bool
}

type FieldInfo struct {
	Name, GoName, GoType                          string
	IsID, IsIndexed, IsUnique, IsEnum, IsRepeated bool
}

func ExtractMessageInfo(msg *protogen.Message, config *EntityConfig) MessageInfo {
	fields := Map(msg.Fields, func(f *protogen.Field) FieldInfo {
		return ExtractFieldInfo(f, config.IDField)
	})

	hasID, hasCreatedAt, hasUpdatedAt, hasDeletedAt := false, false, false, false
	var idGoName string
	for _, f := range fields {
		snake := toSnakeCase(f.Name)
		switch {
		case f.IsID:
			hasID = true
			idGoName = f.GoName
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
		Collection:   config.Collection,
		IDField:      config.IDField,
		IDGoName:     idGoName,
		Fields:       fields,
		HasID:        hasID,
		HasCreatedAt: hasCreatedAt,
		HasUpdatedAt: hasUpdatedAt,
		HasDeletedAt: hasDeletedAt,
	}
}

func ExtractFieldInfo(field *protogen.Field, idField string) FieldInfo {
	name := string(field.Desc.Name())
	isID := strings.EqualFold(name, idField)
	isUnique := isID || strings.EqualFold(name, "email") || strings.EqualFold(name, "slug") || strings.EqualFold(name, "username")
	isIndexed := isUnique || field.Desc.Kind() == protoreflect.EnumKind ||
		strings.HasSuffix(strings.ToLower(name), "_id") ||
		strings.EqualFold(name, "status") || strings.EqualFold(name, "role")
	isRepeated := field.Desc.IsList()
	goType := fieldGoType(field)
	if isRepeated {
		goType = "[]" + goType
	}
	return FieldInfo{
		Name: name, GoName: field.GoName, GoType: goType,
		IsID: isID, IsIndexed: isIndexed, IsUnique: isUnique,
		IsEnum: field.Desc.Kind() == protoreflect.EnumKind, IsRepeated: isRepeated,
	}
}

func fieldGoType(field *protogen.Field) string {
	switch field.Desc.Kind() {
	case protoreflect.BoolKind:
		return "bool"
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return "int32"
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return "int64"
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return "uint32"
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return "uint64"
	case protoreflect.FloatKind:
		return "float32"
	case protoreflect.DoubleKind:
		return "float64"
	case protoreflect.StringKind:
		return "string"
	case protoreflect.BytesKind:
		return "[]byte"
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
// FIRESTORE GENERATORS (Monoid composition)
// =============================================================================

func Header() Code {
	return Comment("Code generated by protoc-gen-firestore. DO NOT EDIT.")
}

func CommonErrors() Code {
	return Concat(CodeMonoid, []Code{
		Blank(), VarBlock(Concat(CodeMonoid, []Code{
			Line("ErrNotFound = errors.New(\"not found\")"),
			Line("ErrInvalidID = errors.New(\"invalid id\")"),
			Line("ErrAlreadyExists = errors.New(\"already exists\")"),
		})),
	})
}

func RepositoryStruct(m MessageInfo) Code {
	return Concat(CodeMonoid, []Code{
		Blank(),
		Struct("Firestore"+m.GoName+"Repository", Field("client", "*firestore.Client")),
	})
}

func Constructor(m MessageInfo) Code {
	return Concat(CodeMonoid, []Code{
		Blank(),
		Func("NewFirestore"+m.GoName+"Repository", "client *firestore.Client", "*Firestore"+m.GoName+"Repository",
			Return("&Firestore"+m.GoName+"Repository{client: client}")),
	})
}

func CollectionHelpers(m MessageInfo) Code {
	recv := "r *Firestore" + m.GoName + "Repository"
	return Concat(CodeMonoid, []Code{
		Blank(), Method(recv, "Collection", "", "*firestore.CollectionRef", Return(fmt.Sprintf("r.client.Collection(%q)", m.Collection))),
		Blank(), Method(recv, "Doc", "id string", "*firestore.DocumentRef", Return("r.Collection().Doc(id)")),
	})
}

func CreateMethod(m MessageInfo) Code {
	recv := "r *Firestore" + m.GoName + "Repository"
	return Concat(CodeMonoid, []Code{
		Blank(), Comment("Create adds a new " + m.GoName + " to Firestore"),
		Method(recv, "Create", "ctx context.Context, entity *"+m.GoName, "error",
			Concat(CodeMonoid, []Code{
				When(m.HasCreatedAt || m.HasUpdatedAt, Concat(CodeMonoid, []Code{
					Line("now := timestamppb.Now()"),
					When(m.HasCreatedAt, Line("entity.CreatedAt = now")),
					When(m.HasUpdatedAt, Line("entity.UpdatedAt = now")),
				})),
				IfElse(fmt.Sprintf("entity.%s == \"\"", m.IDGoName),
					Concat(CodeMonoid, []Code{
						Line("ref := r.Collection().NewDoc()"),
						Linef("entity.%s = ref.ID", m.IDGoName),
						Line("_, err := ref.Set(ctx, r.toFirestoreData(entity))"),
						Return("err"),
					}),
					Concat(CodeMonoid, []Code{
						Linef("_, err := r.Doc(entity.%s).Set(ctx, r.toFirestoreData(entity))", m.IDGoName),
						Return("err"),
					})),
			})),
	})
}

func GetMethod(m MessageInfo) Code {
	recv := "r *Firestore" + m.GoName + "Repository"
	return Concat(CodeMonoid, []Code{
		Blank(), Comment("Get retrieves a " + m.GoName + " by ID"),
		Method(recv, "Get", "ctx context.Context, id string", "(*"+m.GoName+", error)",
			Concat(CodeMonoid, []Code{
				Line("doc, err := r.Doc(id).Get(ctx)"),
				If("status.Code(err) == codes.NotFound", Return("nil, ErrNotFound")),
				If("err != nil", Return("nil, err")),
				Return("r.fromFirestoreDoc(doc)"),
			})),
	})
}

func UpdateMethod(m MessageInfo) Code {
	recv := "r *Firestore" + m.GoName + "Repository"
	return Concat(CodeMonoid, []Code{
		Blank(), Comment("Update modifies an existing " + m.GoName),
		Method(recv, "Update", "ctx context.Context, entity *"+m.GoName, "error",
			Concat(CodeMonoid, []Code{
				If(fmt.Sprintf("entity.%s == \"\"", m.IDGoName), Return("ErrInvalidID")),
				When(m.HasUpdatedAt, Line("entity.UpdatedAt = timestamppb.Now()")),
				Linef("_, err := r.Doc(entity.%s).Set(ctx, r.toFirestoreData(entity))", m.IDGoName),
				Return("err"),
			})),
	})
}

func DeleteMethod(m MessageInfo) Code {
	recv := "r *Firestore" + m.GoName + "Repository"
	return Concat(CodeMonoid, []Code{
		Blank(), Comment("Delete removes a " + m.GoName + " by ID"),
		Method(recv, "Delete", "ctx context.Context, id string", "error",
			Concat(CodeMonoid, []Code{
				If(`id == ""`, Return("ErrInvalidID")),
				Line("_, err := r.Doc(id).Delete(ctx)"),
				Return("err"),
			})),
	})
}

func SoftDeleteMethods(m MessageInfo) Code {
	if !m.HasDeletedAt {
		return CodeMonoid.Empty()
	}
	recv := "r *Firestore" + m.GoName + "Repository"
	return Concat(CodeMonoid, []Code{
		Blank(), Comment("SoftDelete marks entity as deleted without removing"),
		Method(recv, "SoftDelete", "ctx context.Context, id string", "error",
			Concat(CodeMonoid, []Code{
				If(`id == ""`, Return("ErrInvalidID")),
				Line("_, err := r.Doc(id).Update(ctx, []firestore.Update{{Path: \"deleted_at\", Value: timestamppb.Now()}})"),
				Return("err"),
			})),
		Blank(), Method(recv, "Restore", "ctx context.Context, id string", "error",
			Concat(CodeMonoid, []Code{
				If(`id == ""`, Return("ErrInvalidID")),
				Line("_, err := r.Doc(id).Update(ctx, []firestore.Update{{Path: \"deleted_at\", Value: nil}})"),
				Return("err"),
			})),
	})
}

func ListMethod(m MessageInfo) Code {
	recv := "r *Firestore" + m.GoName + "Repository"
	return Concat(CodeMonoid, []Code{
		Blank(), Comment("List retrieves all " + m.GoName + "s with optional limit"),
		Method(recv, "List", "ctx context.Context, limit int", "([]*"+m.GoName+", error)",
			Concat(CodeMonoid, []Code{
				Line("q := r.Collection().Query"),
				When(m.HasDeletedAt, Line("q = q.Where(\"deleted_at\", \"==\", nil)")),
				If("limit > 0", Line("q = q.Limit(limit)")),
				Line("iter := q.Documents(ctx)"),
				Line("defer iter.Stop()"),
				Linef("var results []*%s", m.GoName),
				Line("for {"),
				Line("\tdoc, err := iter.Next()"),
				If("err == iterator.Done", Line("break")),
				If("err != nil", Return("nil, err")),
				Line("\te, err := r.fromFirestoreDoc(doc)"),
				If("err != nil", Return("nil, err")),
				Line("\tresults = append(results, e)"),
				Line("}"),
				Return("results, nil"),
			})),
	})
}

func ExistsMethod(m MessageInfo) Code {
	recv := "r *Firestore" + m.GoName + "Repository"
	return Concat(CodeMonoid, []Code{
		Blank(), Comment("Exists checks if a " + m.GoName + " exists"),
		Method(recv, "Exists", "ctx context.Context, id string", "(bool, error)",
			Concat(CodeMonoid, []Code{
				Line("doc, err := r.Doc(id).Get(ctx)"),
				If("status.Code(err) == codes.NotFound", Return("false, nil")),
				If("err != nil", Return("false, err")),
				Return("doc.Exists(), nil"),
			})),
	})
}

func CountMethod(m MessageInfo) Code {
	recv := "r *Firestore" + m.GoName + "Repository"
	return Concat(CodeMonoid, []Code{
		Blank(), Comment("Count returns the number of " + m.GoName + "s"),
		Method(recv, "Count", "ctx context.Context", "(int, error)",
			Concat(CodeMonoid, []Code{
				Line("q := r.Collection().Query"),
				When(m.HasDeletedAt, Line("q = q.Where(\"deleted_at\", \"==\", nil)")),
				Line("docs, err := q.Documents(ctx).GetAll()"),
				If("err != nil", Return("0, err")),
				Return("len(docs), nil"),
			})),
	})
}

func FindMethods(m MessageInfo) Code {
	indexedFields := Filter(m.Fields, func(f FieldInfo) bool { return f.IsIndexed && !f.IsID })
	if len(indexedFields) == 0 {
		return CodeMonoid.Empty()
	}
	recv := "r *Firestore" + m.GoName + "Repository"
	return FoldMap(indexedFields, CodeMonoid, func(f FieldInfo) Code {
		methodName := "FindBy" + f.GoName
		paramType := f.GoType
		if f.IsRepeated {
			paramType = strings.TrimPrefix(paramType, "[]")
		}
		return Concat(CodeMonoid, []Code{
			Blank(), Commentf("%s finds %ss by %s", methodName, m.GoName, f.Name),
			Method(recv, methodName, "ctx context.Context, value "+paramType, "([]*"+m.GoName+", error)",
				Concat(CodeMonoid, []Code{
					Linef("q := r.Collection().Where(%q, \"==\", value)", toSnakeCase(f.Name)),
					When(m.HasDeletedAt, Line("q = q.Where(\"deleted_at\", \"==\", nil)")),
					Line("iter := q.Documents(ctx)"),
					Line("defer iter.Stop()"),
					Linef("var results []*%s", m.GoName),
					Line("for {"),
					Line("\tdoc, err := iter.Next()"),
					If("err == iterator.Done", Line("break")),
					If("err != nil", Return("nil, err")),
					Line("\te, err := r.fromFirestoreDoc(doc)"),
					If("err != nil", Return("nil, err")),
					Line("\tresults = append(results, e)"),
					Line("}"),
					Return("results, nil"),
				})),
		})
	})
}

func BatchMethods(m MessageInfo) Code {
	recv := "r *Firestore" + m.GoName + "Repository"
	return Concat(CodeMonoid, []Code{
		Blank(), Comment("=== Batch Operations ==="),
		Blank(), Method(recv, "CreateBatch", "ctx context.Context, entities []*"+m.GoName, "error",
			Concat(CodeMonoid, []Code{
				If("len(entities) == 0", Return("nil")),
				If("len(entities) > 500", Return("fmt.Errorf(\"batch size exceeds 500\")")),
				Line("batch := r.client.Batch()"),
				When(m.HasCreatedAt || m.HasUpdatedAt, Line("now := timestamppb.Now()")),
				Line("for _, entity := range entities {"),
				When(m.HasCreatedAt, Line("\tentity.CreatedAt = now")),
				When(m.HasUpdatedAt, Line("\tentity.UpdatedAt = now")),
				Linef("\tif entity.%s == \"\" {", m.IDGoName),
				Line("\t\tref := r.Collection().NewDoc()"),
				Linef("\t\tentity.%s = ref.ID", m.IDGoName),
				Line("\t\tbatch.Set(ref, r.toFirestoreData(entity))"),
				Line("\t} else {"),
				Linef("\t\tbatch.Set(r.Doc(entity.%s), r.toFirestoreData(entity))", m.IDGoName),
				Line("\t}"),
				Line("}"),
				Line("_, err := batch.Commit(ctx)"),
				Return("err"),
			})),
		Blank(), Method(recv, "DeleteBatch", "ctx context.Context, ids []string", "error",
			Concat(CodeMonoid, []Code{
				If("len(ids) == 0", Return("nil")),
				If("len(ids) > 500", Return("fmt.Errorf(\"batch size exceeds 500\")")),
				Line("batch := r.client.Batch()"),
				Line("for _, id := range ids {"),
				Line("\tbatch.Delete(r.Doc(id))"),
				Line("}"),
				Line("_, err := batch.Commit(ctx)"),
				Return("err"),
			})),
	})
}

func QueryBuilder(m MessageInfo) Code {
	recv := "r *Firestore" + m.GoName + "Repository"
	qName := m.GoName + "Query"
	qRecv := "q *" + qName
	return Concat(CodeMonoid, []Code{
		Blank(), Comment("=== Query Builder ==="),
		Blank(), Struct(qName, Concat(CodeMonoid, []Code{
			Field("repo", "*Firestore"+m.GoName+"Repository"),
			Field("query", "firestore.Query"),
			Field("limit", "int"),
			Field("offset", "int"),
		})),
		Blank(), Method(recv, "Query", "", "*"+qName,
			Concat(CodeMonoid, []Code{
				Line("q := r.Collection().Query"),
				When(m.HasDeletedAt, Line("q = q.Where(\"deleted_at\", \"==\", nil)")),
				Return("&" + qName + "{repo: r, query: q}"),
			})),
		Blank(), Method(qRecv, "Where", "field string, op string, value interface{}", "*"+qName, Concat(CodeMonoid, []Code{Line("q.query = q.query.Where(field, op, value)"), Return("q")})),
		Blank(), Method(qRecv, "OrderBy", "field string, dir firestore.Direction", "*"+qName, Concat(CodeMonoid, []Code{Line("q.query = q.query.OrderBy(field, dir)"), Return("q")})),
		Blank(), Method(qRecv, "Limit", "n int", "*"+qName, Concat(CodeMonoid, []Code{Line("q.limit = n"), Return("q")})),
		Blank(), Method(qRecv, "Offset", "n int", "*"+qName, Concat(CodeMonoid, []Code{Line("q.offset = n"), Return("q")})),
		Blank(), Method(qRecv, "Get", "ctx context.Context", "([]*"+m.GoName+", error)",
			Concat(CodeMonoid, []Code{
				Line("q := q.query"),
				If("q.limit > 0", Line("q = q.Limit(q.limit)")),
				If("q.offset > 0", Line("q = q.Offset(q.offset)")),
				Line("iter := q.Documents(ctx)"),
				Line("defer iter.Stop()"),
				Linef("var results []*%s", m.GoName),
				Line("for {"),
				Line("\tdoc, err := iter.Next()"),
				If("err == iterator.Done", Line("break")),
				If("err != nil", Return("nil, err")),
				Line("\te, err := q.repo.fromFirestoreDoc(doc)"),
				If("err != nil", Return("nil, err")),
				Line("\tresults = append(results, e)"),
				Line("}"),
				Return("results, nil"),
			})),
		Blank(), Method(qRecv, "First", "ctx context.Context", "(*"+m.GoName+", error)",
			Concat(CodeMonoid, []Code{
				Line("q.limit = 1"),
				Line("results, err := q.Get(ctx)"),
				If("err != nil", Return("nil, err")),
				If("len(results) == 0", Return("nil, ErrNotFound")),
				Return("results[0], nil"),
			})),
	})
}

func TransactionHelpers(m MessageInfo) Code {
	recv := "r *Firestore" + m.GoName + "Repository"
	txName := m.GoName + "Tx"
	return Concat(CodeMonoid, []Code{
		Blank(), Comment("=== Transaction Support ==="),
		Blank(), Method(recv, "RunTransaction", "ctx context.Context, fn func(context.Context, *"+txName+") error", "error",
			Concat(CodeMonoid, []Code{Line("return r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {"), Linef("\treturn fn(ctx, &%s{repo: r, tx: tx})", txName), Line("})")})),
		Blank(), Struct(txName, Concat(CodeMonoid, []Code{Field("repo", "*Firestore"+m.GoName+"Repository"), Field("tx", "*firestore.Transaction")})),
		Blank(), Method("t *"+txName, "Get", "id string", "(*"+m.GoName+", error)",
			Concat(CodeMonoid, []Code{
				Line("doc, err := t.tx.Get(t.repo.Doc(id))"),
				If("status.Code(err) == codes.NotFound", Return("nil, ErrNotFound")),
				If("err != nil", Return("nil, err")),
				Return("t.repo.fromFirestoreDoc(doc)"),
			})),
		Blank(), Method("t *"+txName, "Create", "entity *"+m.GoName, "error",
			Concat(CodeMonoid, []Code{
				When(m.HasCreatedAt || m.HasUpdatedAt, Concat(CodeMonoid, []Code{
					Line("now := timestamppb.Now()"),
					When(m.HasCreatedAt, Line("entity.CreatedAt = now")),
					When(m.HasUpdatedAt, Line("entity.UpdatedAt = now")),
				})),
				IfElse(fmt.Sprintf("entity.%s == \"\"", m.IDGoName),
					Concat(CodeMonoid, []Code{Line("ref := t.repo.Collection().NewDoc()"), Linef("entity.%s = ref.ID", m.IDGoName), Return("t.tx.Create(ref, t.repo.toFirestoreData(entity))")}),
					Return(fmt.Sprintf("t.tx.Create(t.repo.Doc(entity.%s), t.repo.toFirestoreData(entity))", m.IDGoName))),
			})),
		Blank(), Method("t *"+txName, "Update", "entity *"+m.GoName, "error",
			Concat(CodeMonoid, []Code{
				If(fmt.Sprintf("entity.%s == \"\"", m.IDGoName), Return("ErrInvalidID")),
				When(m.HasUpdatedAt, Line("entity.UpdatedAt = timestamppb.Now()")),
				Return(fmt.Sprintf("t.tx.Set(t.repo.Doc(entity.%s), t.repo.toFirestoreData(entity))", m.IDGoName)),
			})),
		Blank(), Method("t *"+txName, "Delete", "id string", "error", Concat(CodeMonoid, []Code{If(`id == ""`, Return("ErrInvalidID")), Return("t.tx.Delete(t.repo.Doc(id))")})),
	})
}

func Converters(m MessageInfo) Code {
	recv := "r *Firestore" + m.GoName + "Repository"
	return Concat(CodeMonoid, []Code{
		Blank(), Comment("=== Converters ==="),
		Blank(), Method(recv, "toFirestoreData", "entity *"+m.GoName, "map[string]interface{}",
			Concat(CodeMonoid, []Code{
				Line("data := make(map[string]interface{})"),
				FoldMap(m.Fields, CodeMonoid, func(f FieldInfo) Code {
					if f.IsID {
						return CodeMonoid.Empty()
					}
					return Linef(`data[%q] = entity.%s`, toSnakeCase(f.Name), f.GoName)
				}),
				Return("data"),
			})),
		Blank(), Method(recv, "fromFirestoreDoc", "doc *firestore.DocumentSnapshot", "(*"+m.GoName+", error)",
			Concat(CodeMonoid, []Code{
				If("!doc.Exists()", Return("nil, ErrNotFound")),
				Linef("entity := &%s{%s: doc.Ref.ID}", m.GoName, m.IDGoName),
				Line("data := doc.Data()"),
				FoldMap(m.Fields, CodeMonoid, func(f FieldInfo) Code {
					if f.IsID {
						return CodeMonoid.Empty()
					}
					return fieldConversion(f)
				}),
				Return("entity, nil"),
			})),
	})
}

func fieldConversion(f FieldInfo) Code {
	fsField := toSnakeCase(f.Name)

	// Handle repeated fields (arrays)
	if f.IsRepeated {
		baseType := strings.TrimPrefix(f.GoType, "[]")
		return Concat(CodeMonoid, []Code{
			Linef(`if v, ok := data[%q].([]interface{}); ok {`, fsField),
			Linef("\tfor _, item := range v {"),
			Linef("\t\tif val, ok := item.(%s); ok { entity.%s = append(entity.%s, val) }", baseType, f.GoName, f.GoName),
			Line("\t}"),
			Line("}"),
		})
	}

	switch f.GoType {
	case "*timestamppb.Timestamp":
		return Concat(CodeMonoid, []Code{
			Linef(`if v, ok := data[%q]; ok && v != nil {`, fsField),
			Linef("\tif t, ok := v.(time.Time); ok { entity.%s = timestamppb.New(t) }", f.GoName),
			Line("}"),
		})
	case "string":
		return Linef(`if v, ok := data[%q].(string); ok { entity.%s = v }`, fsField, f.GoName)
	case "int32":
		return Linef(`if v, ok := data[%q].(int64); ok { entity.%s = int32(v) }`, fsField, f.GoName)
	case "int64":
		return Linef(`if v, ok := data[%q].(int64); ok { entity.%s = v }`, fsField, f.GoName)
	case "float64":
		return Linef(`if v, ok := data[%q].(float64); ok { entity.%s = v }`, fsField, f.GoName)
	case "bool":
		return Linef(`if v, ok := data[%q].(bool); ok { entity.%s = v }`, fsField, f.GoName)
	default:
		if f.IsEnum {
			return Linef(`if v, ok := data[%q].(int64); ok { entity.%s = %s(v) }`, fsField, f.GoName, f.GoType)
		}
		return CodeMonoid.Empty()
	}
}

// =============================================================================
// MAIN COMPOSITION - FoldMap over messages!
// =============================================================================

func MessageRepository(m MessageInfo) Code {
	return Concat(CodeMonoid, []Code{
		Blank(),
		Linef("// ============================================================================"),
		Linef("// %s Repository - CRUD + Find Methods", m.GoName),
		Linef("// ============================================================================"),
		RepositoryStruct(m), Constructor(m), CollectionHelpers(m),
		CreateMethod(m), GetMethod(m), UpdateMethod(m), DeleteMethod(m),
		SoftDeleteMethods(m), ListMethod(m), ExistsMethod(m), CountMethod(m),
		FindMethods(m), BatchMethods(m), QueryBuilder(m), TransactionHelpers(m), Converters(m),
	})
}

func GenerateFile(file *protogen.File, entityMessages []*protogen.Message, configs map[string]*EntityConfig) Code {
	if len(entityMessages) == 0 {
		return CodeMonoid.Empty()
	}

	messages := Map(entityMessages, func(msg *protogen.Message) MessageInfo {
		return ExtractMessageInfo(msg, configs[string(msg.Desc.Name())])
	})

	return Concat(CodeMonoid, []Code{
		Header(), Blank(), Package(string(file.GoPackageName)),
		Imports("context", "fmt", "time", "", "cloud.google.com/go/firestore",
			"google.golang.org/api/iterator", "google.golang.org/grpc/codes",
			"google.golang.org/grpc/status", "google.golang.org/protobuf/types/known/timestamppb"),
		FoldMap(messages, CodeMonoid, MessageRepository),
	})
}

func GenerateErrorsFile(pkgName string) Code {
	return Concat(CodeMonoid, []Code{
		Header(), Blank(), Package(pkgName),
		Imports("errors"),
		CommonErrors(),
	})
}

func main() {
	protogen.Options{}.Run(func(gen *protogen.Plugin) error {
		gen.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)
		errorsGenerated := false

		for _, f := range gen.Files {
			if !f.Generate || len(f.Messages) == 0 {
				continue
			}

			// Collect entity messages (those with entity option)
			var entityMessages []*protogen.Message
			configs := make(map[string]*EntityConfig)

			for _, msg := range f.Messages {
				config := getEntityConfig(msg)
				if config != nil && config.Generate {
					entityMessages = append(entityMessages, msg)
					configs[string(msg.Desc.Name())] = config
				}
			}

			if len(entityMessages) == 0 {
				continue
			}

			// Generate errors file only once per package
			if !errorsGenerated {
				errFile := gen.NewGeneratedFile(f.GeneratedFilenamePrefix+"_errors.pb.go", f.GoImportPath)
				errFile.P(GenerateErrorsFile(string(f.GoPackageName)).Run())
				errorsGenerated = true
			}

			g := gen.NewGeneratedFile(f.GeneratedFilenamePrefix+"_firestore.pb.go", f.GoImportPath)
			g.P(GenerateFile(f, entityMessages, configs).Run())
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
