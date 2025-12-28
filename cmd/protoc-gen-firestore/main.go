// protoc-gen-firestore generates Firestore CRUD stubs using Category Theory composition
// No string append - uses: Monoid, Functor (Map), Fold, When
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
// MESSAGE INFO (Pure data extraction)
// =============================================================================

type MessageInfo struct {
	Name, GoName, Collection                        string
	Fields                                          []FieldInfo
	HasID, HasCreatedAt, HasUpdatedAt, HasDeletedAt bool
}

type FieldInfo struct {
	Name, GoName, GoType                          string
	IsID, IsIndexed, IsUnique, IsEnum, IsRepeated bool
}

func ExtractMessageInfo(msg *protogen.Message) MessageInfo {
	fields := Map(msg.Fields, ExtractFieldInfo)
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
		Name: string(msg.Desc.Name()), GoName: msg.GoIdent.GoName,
		Collection: toSnakeCase(string(msg.Desc.Name())) + "s", Fields: fields,
		HasID: hasID, HasCreatedAt: hasCreatedAt, HasUpdatedAt: hasUpdatedAt, HasDeletedAt: hasDeletedAt,
	}
}

func ExtractFieldInfo(field *protogen.Field) FieldInfo {
	name := string(field.Desc.Name())
	isUnique := strings.EqualFold(name, "email") || strings.EqualFold(name, "slug") || strings.EqualFold(name, "username")
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
		IsID: strings.EqualFold(name, "id"), IsIndexed: isIndexed, IsUnique: isUnique,
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
	return Concat(CodeMonoid, []Code{
		Comment("Code generated by protoc-gen-firestore. DO NOT EDIT."),
		Comment("Generated using Category Theory: Monoid + Functor + Fold"),
	})
}

func CommonErrors() Code {
	return Concat(CodeMonoid, []Code{
		Blank(), Comment("Common errors"),
		VarBlock(Concat(CodeMonoid, []Code{
			Line(`ErrNotFound      = errors.New("document not found")`),
			Line(`ErrAlreadyExists = errors.New("document already exists")`),
			Line(`ErrInvalidID     = errors.New("invalid document ID")`),
		})),
	})
}

func RepositoryStruct(m MessageInfo) Code {
	return Concat(CodeMonoid, []Code{
		Blank(), Commentf("%sRepository interface for %s persistence", m.GoName, m.GoName),
		Linef("type %sRepository interface {", m.GoName),
		Linef("	Create(ctx context.Context, entity *%s) (string, error)", m.GoName),
		Linef("	Get(ctx context.Context, id string) (*%s, error)", m.GoName),
		Linef("	Update(ctx context.Context, entity *%s) error", m.GoName),
		Line("	Delete(ctx context.Context, id string) error"),
		Linef("	List(ctx context.Context) ([]*%s, error)", m.GoName),
		Line("}"),
		Blank(), Commentf("Firestore%sRepository implements %sRepository using Firestore", m.GoName, m.GoName),
		Struct("Firestore"+m.GoName+"Repository", Concat(CodeMonoid, []Code{
			Field("client", "*firestore.Client"),
			Field("collection", "string"),
		})),
	})
}

func Constructor(m MessageInfo) Code {
	return Concat(CodeMonoid, []Code{
		Blank(), Commentf("NewFirestore%sRepository creates a new Firestore repository", m.GoName),
		Func("NewFirestore"+m.GoName+"Repository", "client *firestore.Client", "*Firestore"+m.GoName+"Repository",
			Return(fmt.Sprintf("&Firestore%sRepository{client: client, collection: %q}", m.GoName, m.Collection))),
	})
}

func CollectionHelpers(m MessageInfo) Code {
	recv := "r *Firestore" + m.GoName + "Repository"
	return Concat(CodeMonoid, []Code{
		Blank(), Method(recv, "Collection", "", "*firestore.CollectionRef", Return("r.client.Collection(r.collection)")),
		Blank(), Method(recv, "Doc", "id string", "*firestore.DocumentRef", Return("r.Collection().Doc(id)")),
	})
}

func CreateMethod(m MessageInfo) Code {
	recv := "r *Firestore" + m.GoName + "Repository"
	return Concat(CodeMonoid, []Code{
		Blank(), Commentf("Create creates a new %s", m.GoName),
		Method(recv, "Create", "ctx context.Context, entity *"+m.GoName, "(string, error)",
			Concat(CodeMonoid, []Code{
				If("entity == nil", Return(`"", errors.New("entity cannot be nil")`)),
				Blank(),
				When(m.HasCreatedAt || m.HasUpdatedAt, Concat(CodeMonoid, []Code{
					Line("now := timestamppb.Now()"),
					When(m.HasCreatedAt, Line("entity.CreatedAt = now")),
					When(m.HasUpdatedAt, Line("entity.UpdatedAt = now")),
					Blank(),
				})),
				Line("data := r.toFirestoreData(entity)"),
				Line("var docRef *firestore.DocumentRef"),
				IfElse(`entity.Id != ""`,
					Concat(CodeMonoid, []Code{
						Line("docRef = r.Doc(entity.Id)"),
						Line("_, err := docRef.Create(ctx, data)"),
						If("err != nil", Concat(CodeMonoid, []Code{
							If("status.Code(err) == codes.AlreadyExists", Return(`"", ErrAlreadyExists`)),
							Return(`"", fmt.Errorf("create failed: %w", err)`),
						})),
					}),
					Concat(CodeMonoid, []Code{
						Line("var err error"),
						Line("docRef, _, err = r.Collection().Add(ctx, data)"),
						If("err != nil", Return(`"", fmt.Errorf("create failed: %w", err)`)),
						Line("entity.Id = docRef.ID"),
					})),
				Return("docRef.ID, nil"),
			})),
	})
}

func GetMethod(m MessageInfo) Code {
	recv := "r *Firestore" + m.GoName + "Repository"
	return Concat(CodeMonoid, []Code{
		Blank(), Commentf("Get retrieves a %s by ID", m.GoName),
		Method(recv, "Get", "ctx context.Context, id string", "(*"+m.GoName+", error)",
			Concat(CodeMonoid, []Code{
				If(`id == ""`, Return("nil, ErrInvalidID")),
				Line("doc, err := r.Doc(id).Get(ctx)"),
				If("err != nil", Concat(CodeMonoid, []Code{
					If("status.Code(err) == codes.NotFound", Return("nil, ErrNotFound")),
					Return(`nil, fmt.Errorf("get failed: %w", err)`),
				})),
				Return("r.fromFirestoreDoc(doc)"),
			})),
		Blank(), Commentf("GetOrNil returns nil if not found"),
		Method(recv, "GetOrNil", "ctx context.Context, id string", "(*"+m.GoName+", error)",
			Concat(CodeMonoid, []Code{
				Line("entity, err := r.Get(ctx, id)"),
				If("errors.Is(err, ErrNotFound)", Return("nil, nil")),
				Return("entity, err"),
			})),
	})
}

func UpdateMethod(m MessageInfo) Code {
	recv := "r *Firestore" + m.GoName + "Repository"
	return Concat(CodeMonoid, []Code{
		Blank(), Commentf("Update updates an existing %s", m.GoName),
		Method(recv, "Update", "ctx context.Context, entity *"+m.GoName, "error",
			Concat(CodeMonoid, []Code{
				If("entity == nil", Return(`errors.New("entity cannot be nil")`)),
				If(`entity.Id == ""`, Return("ErrInvalidID")),
				When(m.HasUpdatedAt, Line("entity.UpdatedAt = timestamppb.Now()")),
				Line("_, err := r.Doc(entity.Id).Set(ctx, r.toFirestoreData(entity))"),
				Return("err"),
			})),
		Blank(), Commentf("UpdateFields updates specific fields"),
		Method(recv, "UpdateFields", "ctx context.Context, id string, updates map[string]interface{}", "error",
			Concat(CodeMonoid, []Code{
				If(`id == ""`, Return("ErrInvalidID")),
				When(m.HasUpdatedAt, Line(`updates["updated_at"] = firestore.ServerTimestamp`)),
				Line("paths := make([]firestore.Update, 0, len(updates))"),
				Line("for k, v := range updates { paths = append(paths, firestore.Update{Path: k, Value: v}) }"),
				Line("_, err := r.Doc(id).Update(ctx, paths)"),
				If("status.Code(err) == codes.NotFound", Return("ErrNotFound")),
				Return("err"),
			})),
		Blank(), Commentf("Upsert creates or updates"),
		Method(recv, "Upsert", "ctx context.Context, entity *"+m.GoName, "error",
			Concat(CodeMonoid, []Code{
				If("entity == nil", Return(`errors.New("entity cannot be nil")`)),
				When(m.HasCreatedAt, If("entity.CreatedAt == nil", Line("entity.CreatedAt = timestamppb.Now()"))),
				When(m.HasUpdatedAt, Line("entity.UpdatedAt = timestamppb.Now()")),
				IfElse(`entity.Id == ""`,
					Concat(CodeMonoid, []Code{
						Line("docRef, _, err := r.Collection().Add(ctx, r.toFirestoreData(entity))"),
						If("err != nil", Return("err")),
						Line("entity.Id = docRef.ID"),
						Return("nil"),
					}),
					Concat(CodeMonoid, []Code{
						Line("_, err := r.Doc(entity.Id).Set(ctx, r.toFirestoreData(entity))"),
						Return("err"),
					})),
			})),
	})
}

func DeleteMethod(m MessageInfo) Code {
	recv := "r *Firestore" + m.GoName + "Repository"
	return Concat(CodeMonoid, []Code{
		Blank(), Commentf("Delete permanently deletes a %s", m.GoName),
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
		Blank(), Commentf("SoftDelete marks %s as deleted", m.GoName),
		Method(recv, "SoftDelete", "ctx context.Context, id string", "error",
			Concat(CodeMonoid, []Code{
				If(`id == ""`, Return("ErrInvalidID")),
				Line("_, err := r.Doc(id).Update(ctx, []firestore.Update{"),
				Line(`	{Path: "deleted_at", Value: firestore.ServerTimestamp},`),
				When(m.HasUpdatedAt, Line(`	{Path: "updated_at", Value: firestore.ServerTimestamp},`)),
				Line("})"),
				If("status.Code(err) == codes.NotFound", Return("ErrNotFound")),
				Return("err"),
			})),
		Blank(), Commentf("Restore restores soft-deleted %s", m.GoName),
		Method(recv, "Restore", "ctx context.Context, id string", "error",
			Concat(CodeMonoid, []Code{
				If(`id == ""`, Return("ErrInvalidID")),
				Line(`_, err := r.Doc(id).Update(ctx, []firestore.Update{{Path: "deleted_at", Value: nil}})`),
				Return("err"),
			})),
	})
}

func ListMethod(m MessageInfo) Code {
	recv := "r *Firestore" + m.GoName + "Repository"
	return Concat(CodeMonoid, []Code{
		Blank(), Commentf("List retrieves all %s", m.GoName),
		Method(recv, "List", "ctx context.Context", "([]*"+m.GoName+", error)",
			Concat(CodeMonoid, []Code{
				Line("query := r.Collection().Query"),
				When(m.HasDeletedAt, Line(`query = query.Where("deleted_at", "==", nil)`)),
				Line("iter := query.Documents(ctx)"),
				Line("defer iter.Stop()"),
				Linef("var results []*%s", m.GoName),
				Line("for {"),
				Line("\tdoc, err := iter.Next()"),
				If("err == iterator.Done", Line("break")),
				If("err != nil", Return("nil, err")),
				Line("\tentity, err := r.fromFirestoreDoc(doc)"),
				If("err != nil", Return("nil, err")),
				Line("\tresults = append(results, entity)"),
				Line("}"),
				Return("results, nil"),
			})),
	})
}

func ExistsMethod(m MessageInfo) Code {
	recv := "r *Firestore" + m.GoName + "Repository"
	return Concat(CodeMonoid, []Code{
		Blank(), Commentf("Exists checks if %s exists", m.GoName),
		Method(recv, "Exists", "ctx context.Context, id string", "(bool, error)",
			Concat(CodeMonoid, []Code{
				If(`id == ""`, Return("false, ErrInvalidID")),
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
		Blank(), Commentf("Count returns total %s", m.GoName),
		Method(recv, "Count", "ctx context.Context", "(int64, error)",
			Concat(CodeMonoid, []Code{
				Line("query := r.Collection().Query"),
				When(m.HasDeletedAt, Line(`query = query.Where("deleted_at", "==", nil)`)),
				Line(`agg, err := query.NewAggregationQuery().WithCount("count").Get(ctx)`),
				If("err != nil", Return("0, err")),
				Line(`countResult, ok := agg["count"]`),
				If("!ok || countResult == nil", Return("0, nil")),
				Line("// AggregationResult stores the value directly"),
				Line("intVal, ok := countResult.(*int64)"),
				If("ok && intVal != nil", Return("*intVal, nil")),
				Line("// Try interface conversion"),
				Line("if v, ok := countResult.(int64); ok { return v, nil }"),
				Return("0, nil"),
			})),
	})
}

func FindByFieldMethod(m MessageInfo, f FieldInfo) Code {
	recv := "r *Firestore" + m.GoName + "Repository"
	methodName := "FindBy" + f.GoName
	fsField := toSnakeCase(f.Name)

	if f.IsUnique {
		return Concat(CodeMonoid, []Code{
			Blank(), Commentf("%s finds %s by %s (unique)", methodName, m.GoName, f.Name),
			Method(recv, methodName, fmt.Sprintf("ctx context.Context, %s %s", lowerFirst(f.GoName), f.GoType), "(*"+m.GoName+", error)",
				Concat(CodeMonoid, []Code{
					Linef(`query := r.Collection().Where(%q, "==", %s)`, fsField, lowerFirst(f.GoName)),
					When(m.HasDeletedAt, Line(`query = query.Where("deleted_at", "==", nil)`)),
					Line("query = query.Limit(1)"),
					Line("iter := query.Documents(ctx)"),
					Line("defer iter.Stop()"),
					Line("doc, err := iter.Next()"),
					If("err == iterator.Done", Return("nil, ErrNotFound")),
					If("err != nil", Return("nil, err")),
					Return("r.fromFirestoreDoc(doc)"),
				})),
		})
	}

	return Concat(CodeMonoid, []Code{
		Blank(), Commentf("%s finds all %s by %s", methodName, m.GoName, f.Name),
		Method(recv, methodName, fmt.Sprintf("ctx context.Context, %s %s, limit int", lowerFirst(f.GoName), f.GoType), "([]*"+m.GoName+", error)",
			Concat(CodeMonoid, []Code{
				Linef(`query := r.Collection().Where(%q, "==", %s)`, fsField, lowerFirst(f.GoName)),
				When(m.HasDeletedAt, Line(`query = query.Where("deleted_at", "==", nil)`)),
				If("limit > 0", Line("query = query.Limit(limit)")),
				Line("iter := query.Documents(ctx)"),
				Line("defer iter.Stop()"),
				Linef("var results []*%s", m.GoName),
				Line("for {"),
				Line("\tdoc, err := iter.Next()"),
				If("err == iterator.Done", Line("break")),
				If("err != nil", Return("nil, err")),
				Line("\tentity, err := r.fromFirestoreDoc(doc)"),
				If("err != nil", Return("nil, err")),
				Line("\tresults = append(results, entity)"),
				Line("}"),
				Return("results, nil"),
			})),
	})
}

func FindMethods(m MessageInfo) Code {
	indexed := Filter(m.Fields, func(f FieldInfo) bool { return (f.IsIndexed || f.IsUnique) && !f.IsID })
	return FoldMap(indexed, CodeMonoid, func(f FieldInfo) Code { return FindByFieldMethod(m, f) })
}

func BatchMethods(m MessageInfo) Code {
	recv := "r *Firestore" + m.GoName + "Repository"
	return Concat(CodeMonoid, []Code{
		Blank(), Comment("=== Batch Operations ==="),
		Blank(), Commentf("BatchCreate creates multiple %s", m.GoName),
		Method(recv, "BatchCreate", "ctx context.Context, entities []*"+m.GoName, "([]string, error)",
			Concat(CodeMonoid, []Code{
				If("len(entities) == 0", Return("nil, nil")),
				If("len(entities) > 500", Return(`nil, errors.New("batch exceeds 500")`)),
				Line("batch := r.client.Batch()"),
				Line("ids := make([]string, len(entities))"),
				Line("for i, e := range entities {"),
				When(m.HasCreatedAt || m.HasUpdatedAt, Concat(CodeMonoid, []Code{
					Line("\tnow := timestamppb.Now()"),
					When(m.HasCreatedAt, Line("\te.CreatedAt = now")),
					When(m.HasUpdatedAt, Line("\te.UpdatedAt = now")),
				})),
				Line("\tvar ref *firestore.DocumentRef"),
				IfElse(`e.Id != ""`, Line("\tref = r.Doc(e.Id)"), Concat(CodeMonoid, []Code{Line("\tref = r.Collection().NewDoc()"), Line("\te.Id = ref.ID")})),
				Line("\tbatch.Set(ref, r.toFirestoreData(e))"),
				Line("\tids[i] = ref.ID"),
				Line("}"),
				Line("_, err := batch.Commit(ctx)"),
				Return("ids, err"),
			})),
		Blank(), Commentf("BatchGet retrieves multiple %s", m.GoName),
		Method(recv, "BatchGet", "ctx context.Context, ids []string", "([]*"+m.GoName+", error)",
			Concat(CodeMonoid, []Code{
				If("len(ids) == 0", Return("nil, nil")),
				Line("refs := make([]*firestore.DocumentRef, len(ids))"),
				Line("for i, id := range ids { refs[i] = r.Doc(id) }"),
				Line("docs, err := r.client.GetAll(ctx, refs)"),
				If("err != nil", Return("nil, err")),
				Linef("results := make([]*%s, 0, len(docs))", m.GoName),
				Line("for _, doc := range docs {"),
				If("!doc.Exists()", Line("continue")),
				Line("\te, err := r.fromFirestoreDoc(doc)"),
				If("err != nil", Return("nil, err")),
				Line("\tresults = append(results, e)"),
				Line("}"),
				Return("results, nil"),
			})),
		Blank(), Commentf("BatchDelete deletes multiple %s", m.GoName),
		Method(recv, "BatchDelete", "ctx context.Context, ids []string", "error",
			Concat(CodeMonoid, []Code{
				If("len(ids) == 0", Return("nil")),
				If("len(ids) > 500", Return(`errors.New("batch exceeds 500")`)),
				Line("batch := r.client.Batch()"),
				Line("for _, id := range ids { batch.Delete(r.Doc(id)) }"),
				Line("_, err := batch.Commit(ctx)"),
				Return("err"),
			})),
	})
}

func QueryBuilder(m MessageInfo) Code {
	qn := m.GoName + "Query"
	recv := "q *" + qn
	return Concat(CodeMonoid, []Code{
		Blank(), Comment("=== Query Builder ==="),
		Struct(qn, Concat(CodeMonoid, []Code{Field("repo", "*Firestore"+m.GoName+"Repository"), Field("query", "firestore.Query"), Field("limit", "int")})),
		Blank(), Method("r *Firestore"+m.GoName+"Repository", "Query", "", "*"+qn, Return(fmt.Sprintf("&%s{repo: r, query: r.Collection().Query}", qn))),
		Blank(), Method(recv, "Where", "field, op string, value interface{}", "*"+qn, Concat(CodeMonoid, []Code{Line("q.query = q.query.Where(field, op, value)"), Return("q")})),
		Blank(), Method(recv, "OrderBy", "field string, dir firestore.Direction", "*"+qn, Concat(CodeMonoid, []Code{Line("q.query = q.query.OrderBy(field, dir)"), Return("q")})),
		Blank(), Method(recv, "Limit", "n int", "*"+qn, Concat(CodeMonoid, []Code{Line("q.limit = n"), Return("q")})),
		Blank(), Method(recv, "StartAfter", "doc *firestore.DocumentSnapshot", "*"+qn, Concat(CodeMonoid, []Code{Line("q.query = q.query.StartAfter(doc)"), Return("q")})),
		When(m.HasDeletedAt, Concat(CodeMonoid, []Code{
			Blank(), Method(recv, "ExcludeDeleted", "", "*"+qn, Concat(CodeMonoid, []Code{Line(`q.query = q.query.Where("deleted_at", "==", nil)`), Return("q")})),
		})),
		Blank(), Method(recv, "Get", "ctx context.Context", "([]*"+m.GoName+", error)",
			Concat(CodeMonoid, []Code{
				Line("query := q.query"),
				If("q.limit > 0", Line("query = query.Limit(q.limit)")),
				Line("iter := query.Documents(ctx)"),
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
		Blank(), Method(recv, "First", "ctx context.Context", "(*"+m.GoName+", error)",
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
				IfElse(`entity.Id == ""`,
					Concat(CodeMonoid, []Code{Line("ref := t.repo.Collection().NewDoc()"), Line("entity.Id = ref.ID"), Return("t.tx.Create(ref, t.repo.toFirestoreData(entity))")}),
					Return("t.tx.Create(t.repo.Doc(entity.Id), t.repo.toFirestoreData(entity))")),
			})),
		Blank(), Method("t *"+txName, "Update", "entity *"+m.GoName, "error",
			Concat(CodeMonoid, []Code{
				If(`entity.Id == ""`, Return("ErrInvalidID")),
				When(m.HasUpdatedAt, Line("entity.UpdatedAt = timestamppb.Now()")),
				Return("t.tx.Set(t.repo.Doc(entity.Id), t.repo.toFirestoreData(entity))"),
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
				Linef("entity := &%s{Id: doc.Ref.ID}", m.GoName),
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

func GenerateFile(file *protogen.File) Code {
	// Only process messages that look like entities (have an id field)
	entityMessages := Filter(file.Messages, func(msg *protogen.Message) bool {
		for _, field := range msg.Fields {
			if strings.EqualFold(string(field.Desc.Name()), "id") {
				return true
			}
		}
		return false
	})

	if len(entityMessages) == 0 {
		return CodeMonoid.Empty()
	}

	messages := Map(entityMessages, ExtractMessageInfo)
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

			// Check if file has any entity messages (with id field)
			hasEntities := false
			for _, msg := range f.Messages {
				for _, field := range msg.Fields {
					if strings.EqualFold(string(field.Desc.Name()), "id") {
						hasEntities = true
						break
					}
				}
				if hasEntities {
					break
				}
			}

			if !hasEntities {
				continue
			}

			// Generate errors file only once per package
			if !errorsGenerated {
				errFile := gen.NewGeneratedFile(f.GeneratedFilenamePrefix+"_errors.pb.go", f.GoImportPath)
				errFile.P(GenerateErrorsFile(string(f.GoPackageName)).Run())
				errorsGenerated = true
			}
			g := gen.NewGeneratedFile(f.GeneratedFilenamePrefix+"_firestore.pb.go", f.GoImportPath)
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
