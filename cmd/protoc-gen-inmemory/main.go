// protoc-gen-inmemory generates In-Memory CRUD stubs using Category Theory composition
// No string append - uses: Monoid, Functor (Map), Fold, When
// Perfect for testing, prototyping, or simple applications
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
	Name, GoName, GoType              string
	IsID, IsIndexed, IsUnique, IsEnum bool
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
	return FieldInfo{
		Name: name, GoName: field.GoName, GoType: fieldGoType(field),
		IsID: strings.EqualFold(name, "id"), IsIndexed: isIndexed, IsUnique: isUnique,
		IsEnum: field.Desc.Kind() == protoreflect.EnumKind,
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
// IN-MEMORY GENERATORS (Monoid composition)
// =============================================================================

func Header() Code {
	return Concat(CodeMonoid, []Code{
		Comment("Code generated by protoc-gen-inmemory. DO NOT EDIT."),
		Comment("Generated using Category Theory: Monoid + Functor + Fold"),
		Comment("Thread-safe in-memory storage for testing and prototyping."),
	})
}

func CommonErrors() Code {
	// Interface and errors are defined in firestore - don't redeclare
	return CodeMonoid.Empty()
}

func RepositoryStruct(m MessageInfo) Code {
	return Concat(CodeMonoid, []Code{
		Blank(), Commentf("InMemory%sRepository implements %sRepository using in-memory storage", m.GoName, m.GoName),
		Struct("InMemory"+m.GoName+"Repository", Concat(CodeMonoid, []Code{
			Field("mu", "sync.RWMutex"),
			Linef("data map[string]*%s", m.GoName),
			Comment("Indexes for fast lookups"),
			FoldMap(Filter(m.Fields, func(f FieldInfo) bool { return f.IsUnique && !f.IsID }), CodeMonoid, func(f FieldInfo) Code {
				return Linef("idx%s map[%s]string // %s -> id", f.GoName, f.GoType, toSnakeCase(f.Name))
			}),
		})),
	})
}

func Constructor(m MessageInfo) Code {
	uniqueFields := Filter(m.Fields, func(f FieldInfo) bool { return f.IsUnique && !f.IsID })
	indexInits := FoldMap(uniqueFields, CodeMonoid, func(f FieldInfo) Code {
		return Linef("idx%s: make(map[%s]string),", f.GoName, f.GoType)
	})

	return Concat(CodeMonoid, []Code{
		Blank(), Commentf("NewInMemory%sRepository creates a new in-memory repository", m.GoName),
		Func("NewInMemory"+m.GoName+"Repository", "", "*InMemory"+m.GoName+"Repository",
			Concat(CodeMonoid, []Code{
				Linef("return &InMemory%sRepository{", m.GoName),
				Linef("\tdata: make(map[string]*%s),", m.GoName),
				Indent(indexInits),
				Line("}"),
			})),
	})
}

func CloneMethod(m MessageInfo) Code {
	return Concat(CodeMonoid, []Code{
		Blank(), Comment("clone creates a deep copy to prevent external mutation"),
		Func("(r *InMemory"+m.GoName+"Repository) clone", "entity *"+m.GoName, "*"+m.GoName,
			Concat(CodeMonoid, []Code{
				If("entity == nil", Return("nil")),
				Line("clone := proto.Clone(entity).(*" + m.GoName + ")"),
				Return("clone"),
			})),
	})
}

func CreateMethod(m MessageInfo) Code {
	recv := "r *InMemory" + m.GoName + "Repository"
	uniqueFields := Filter(m.Fields, func(f FieldInfo) bool { return f.IsUnique && !f.IsID })

	uniqueChecks := FoldMap(uniqueFields, CodeMonoid, func(f FieldInfo) Code {
		return Concat(CodeMonoid, []Code{
			Linef("if entity.%s != %s {", f.GoName, zeroValue(f.GoType)),
			Linef("\tif _, exists := r.idx%s[entity.%s]; exists {", f.GoName, f.GoName),
			Linef("\t\treturn \"\", fmt.Errorf(\"%s already exists: %%w\", ErrAlreadyExists)", toSnakeCase(f.Name)),
			Line("\t}"),
			Line("}"),
		})
	})

	indexUpdates := FoldMap(uniqueFields, CodeMonoid, func(f FieldInfo) Code {
		return Concat(CodeMonoid, []Code{
			Linef("if entity.%s != %s {", f.GoName, zeroValue(f.GoType)),
			Linef("\tr.idx%s[entity.%s] = entity.Id", f.GoName, f.GoName),
			Line("}"),
		})
	})

	return Concat(CodeMonoid, []Code{
		Blank(), Commentf("Create creates a new %s", m.GoName),
		Method(recv, "Create", "ctx context.Context, entity *"+m.GoName, "(string, error)",
			Concat(CodeMonoid, []Code{
				If("entity == nil", Return(`"", errors.New("entity cannot be nil")`)),
				Blank(),
				Line("r.mu.Lock()"),
				Line("defer r.mu.Unlock()"),
				Blank(),
				Comment("Generate ID if not provided"),
				IfElse(`entity.Id == ""`,
					Line("entity.Id = uuid.New().String()"),
					Concat(CodeMonoid, []Code{
						If("_, exists := r.data[entity.Id]; exists", Return(`"", ErrAlreadyExists`)),
					})),
				Blank(),
				When(len(uniqueFields) > 0, Concat(CodeMonoid, []Code{
					Comment("Check unique constraints"),
					uniqueChecks,
					Blank(),
				})),
				When(m.HasCreatedAt || m.HasUpdatedAt, Concat(CodeMonoid, []Code{
					Comment("Set timestamps"),
					Line("now := timestamppb.Now()"),
					When(m.HasCreatedAt, Line("entity.CreatedAt = now")),
					When(m.HasUpdatedAt, Line("entity.UpdatedAt = now")),
					Blank(),
				})),
				Comment("Store a clone to prevent external mutation"),
				Line("r.data[entity.Id] = r.clone(entity)"),
				Blank(),
				When(len(uniqueFields) > 0, Concat(CodeMonoid, []Code{
					Comment("Update indexes"),
					indexUpdates,
					Blank(),
				})),
				Return("entity.Id, nil"),
			})),
	})
}

func GetMethod(m MessageInfo) Code {
	recv := "r *InMemory" + m.GoName + "Repository"
	return Concat(CodeMonoid, []Code{
		Blank(), Commentf("Get retrieves a %s by ID", m.GoName),
		Method(recv, "Get", "ctx context.Context, id string", "(*"+m.GoName+", error)",
			Concat(CodeMonoid, []Code{
				If(`id == ""`, Return("nil, ErrInvalidID")),
				Blank(),
				Line("r.mu.RLock()"),
				Line("defer r.mu.RUnlock()"),
				Blank(),
				Line("entity, exists := r.data[id]"),
				If("!exists", Return("nil, ErrNotFound")),
				Blank(),
				When(m.HasDeletedAt, Concat(CodeMonoid, []Code{
					If("entity.DeletedAt != nil", Return("nil, ErrNotFound")),
					Blank(),
				})),
				Return("r.clone(entity), nil"),
			})),
		Blank(), Commentf("GetOrNil returns nil if not found"),
		Method(recv, "GetOrNil", "ctx context.Context, id string", "(*"+m.GoName+", error)",
			Concat(CodeMonoid, []Code{
				Line("entity, err := r.Get(ctx, id)"),
				If("errors.Is(err, ErrNotFound)", Return("nil, nil")),
				Return("entity, err"),
			})),
		Blank(), Commentf("MustGet panics if not found"),
		Method(recv, "MustGet", "ctx context.Context, id string", "*"+m.GoName,
			Concat(CodeMonoid, []Code{
				Line("entity, err := r.Get(ctx, id)"),
				If("err != nil", Line("panic(err)")),
				Return("entity"),
			})),
	})
}

func UpdateMethod(m MessageInfo) Code {
	recv := "r *InMemory" + m.GoName + "Repository"
	uniqueFields := Filter(m.Fields, func(f FieldInfo) bool { return f.IsUnique && !f.IsID })

	indexCleanup := FoldMap(uniqueFields, CodeMonoid, func(f FieldInfo) Code {
		return Concat(CodeMonoid, []Code{
			Linef("if old.%s != %s {", f.GoName, zeroValue(f.GoType)),
			Linef("\tdelete(r.idx%s, old.%s)", f.GoName, f.GoName),
			Line("}"),
		})
	})

	indexUpdates := FoldMap(uniqueFields, CodeMonoid, func(f FieldInfo) Code {
		return Concat(CodeMonoid, []Code{
			Linef("if entity.%s != %s {", f.GoName, zeroValue(f.GoType)),
			Linef("\tr.idx%s[entity.%s] = entity.Id", f.GoName, f.GoName),
			Line("}"),
		})
	})

	// Determine if we need the old value
	needsOld := m.HasDeletedAt || m.HasCreatedAt || len(uniqueFields) > 0

	var getOld Code
	if needsOld {
		getOld = Line("old, exists := r.data[entity.Id]")
	} else {
		getOld = Line("_, exists := r.data[entity.Id]")
	}

	return Concat(CodeMonoid, []Code{
		Blank(), Commentf("Update updates an existing %s", m.GoName),
		Method(recv, "Update", "ctx context.Context, entity *"+m.GoName, "error",
			Concat(CodeMonoid, []Code{
				If("entity == nil", Return(`errors.New("entity cannot be nil")`)),
				If(`entity.Id == ""`, Return("ErrInvalidID")),
				Blank(),
				Line("r.mu.Lock()"),
				Line("defer r.mu.Unlock()"),
				Blank(),
				getOld,
				If("!exists", Return("ErrNotFound")),
				When(m.HasDeletedAt, If("old.DeletedAt != nil", Return("ErrNotFound"))),
				Blank(),
				When(len(uniqueFields) > 0, Concat(CodeMonoid, []Code{
					Comment("Clean up old index entries"),
					indexCleanup,
					Blank(),
				})),
				When(m.HasUpdatedAt, Line("entity.UpdatedAt = timestamppb.Now()")),
				When(m.HasCreatedAt, Line("entity.CreatedAt = old.CreatedAt // Preserve original")),
				Blank(),
				Line("r.data[entity.Id] = r.clone(entity)"),
				Blank(),
				When(len(uniqueFields) > 0, Concat(CodeMonoid, []Code{
					Comment("Update indexes"),
					indexUpdates,
					Blank(),
				})),
				Return("nil"),
			})),
		Blank(), Commentf("Upsert creates or updates a %s", m.GoName),
		Method(recv, "Upsert", "ctx context.Context, entity *"+m.GoName, "error",
			Concat(CodeMonoid, []Code{
				If("entity == nil", Return(`errors.New("entity cannot be nil")`)),
				Blank(),
				IfElse(`entity.Id == ""`,
					Concat(CodeMonoid, []Code{
						Line("_, err := r.Create(ctx, entity)"),
						Return("err"),
					}),
					Concat(CodeMonoid, []Code{
						Line("_, err := r.Get(ctx, entity.Id)"),
						IfElse("errors.Is(err, ErrNotFound)",
							Concat(CodeMonoid, []Code{
								Line("_, err = r.Create(ctx, entity)"),
								Return("err"),
							}),
							Return("r.Update(ctx, entity)")),
					})),
			})),
	})
}

func DeleteMethod(m MessageInfo) Code {
	recv := "r *InMemory" + m.GoName + "Repository"
	uniqueFields := Filter(m.Fields, func(f FieldInfo) bool { return f.IsUnique && !f.IsID })

	indexCleanup := FoldMap(uniqueFields, CodeMonoid, func(f FieldInfo) Code {
		return Concat(CodeMonoid, []Code{
			Linef("if entity.%s != %s {", f.GoName, zeroValue(f.GoType)),
			Linef("\tdelete(r.idx%s, entity.%s)", f.GoName, f.GoName),
			Line("}"),
		})
	})

	return Concat(CodeMonoid, []Code{
		Blank(), Commentf("Delete permanently deletes a %s", m.GoName),
		Method(recv, "Delete", "ctx context.Context, id string", "error",
			Concat(CodeMonoid, []Code{
				If(`id == ""`, Return("ErrInvalidID")),
				Blank(),
				Line("r.mu.Lock()"),
				Line("defer r.mu.Unlock()"),
				Blank(),
				IfElse(fmt.Sprintf("%d > 0", len(uniqueFields)),
					Concat(CodeMonoid, []Code{
						Line("entity, exists := r.data[id]"),
						If("!exists", Return("ErrNotFound")),
						Blank(),
						Comment("Clean up indexes"),
						indexCleanup,
						Blank(),
					}),
					Concat(CodeMonoid, []Code{
						Line("_, exists := r.data[id]"),
						If("!exists", Return("ErrNotFound")),
						Blank(),
					})),
				Line("delete(r.data, id)"),
				Return("nil"),
			})),
	})
}

func SoftDeleteMethods(m MessageInfo) Code {
	if !m.HasDeletedAt {
		return CodeMonoid.Empty()
	}
	recv := "r *InMemory" + m.GoName + "Repository"
	return Concat(CodeMonoid, []Code{
		Blank(), Commentf("SoftDelete marks %s as deleted", m.GoName),
		Method(recv, "SoftDelete", "ctx context.Context, id string", "error",
			Concat(CodeMonoid, []Code{
				If(`id == ""`, Return("ErrInvalidID")),
				Blank(),
				Line("r.mu.Lock()"),
				Line("defer r.mu.Unlock()"),
				Blank(),
				Line("entity, exists := r.data[id]"),
				If("!exists", Return("ErrNotFound")),
				If("entity.DeletedAt != nil", Return("nil // Already deleted")),
				Blank(),
				Line("entity.DeletedAt = timestamppb.Now()"),
				When(m.HasUpdatedAt, Line("entity.UpdatedAt = timestamppb.Now()")),
				Return("nil"),
			})),
		Blank(), Commentf("Restore restores soft-deleted %s", m.GoName),
		Method(recv, "Restore", "ctx context.Context, id string", "error",
			Concat(CodeMonoid, []Code{
				If(`id == ""`, Return("ErrInvalidID")),
				Blank(),
				Line("r.mu.Lock()"),
				Line("defer r.mu.Unlock()"),
				Blank(),
				Line("entity, exists := r.data[id]"),
				If("!exists", Return("ErrNotFound")),
				Blank(),
				Line("entity.DeletedAt = nil"),
				When(m.HasUpdatedAt, Line("entity.UpdatedAt = timestamppb.Now()")),
				Return("nil"),
			})),
		Blank(), Commentf("HardDelete permanently removes a soft-deleted %s", m.GoName),
		Method(recv, "HardDelete", "ctx context.Context, id string", "error",
			Return("r.Delete(ctx, id)")),
	})
}

func ListMethod(m MessageInfo) Code {
	recv := "r *InMemory" + m.GoName + "Repository"
	return Concat(CodeMonoid, []Code{
		Blank(), Commentf("List retrieves all %s", m.GoName),
		Method(recv, "List", "ctx context.Context", "([]*"+m.GoName+", error)",
			Concat(CodeMonoid, []Code{
				Line("r.mu.RLock()"),
				Line("defer r.mu.RUnlock()"),
				Blank(),
				Linef("results := make([]*%s, 0, len(r.data))", m.GoName),
				Line("for _, entity := range r.data {"),
				When(m.HasDeletedAt, Concat(CodeMonoid, []Code{
					If("entity.DeletedAt != nil", Line("continue")),
				})),
				Line("\tresults = append(results, r.clone(entity))"),
				Line("}"),
				Return("results, nil"),
			})),
		Blank(), Commentf("ListAll retrieves all %s including soft-deleted", m.GoName),
		Method(recv, "ListAll", "ctx context.Context", "([]*"+m.GoName+", error)",
			Concat(CodeMonoid, []Code{
				Line("r.mu.RLock()"),
				Line("defer r.mu.RUnlock()"),
				Blank(),
				Linef("results := make([]*%s, 0, len(r.data))", m.GoName),
				Line("for _, entity := range r.data {"),
				Line("\tresults = append(results, r.clone(entity))"),
				Line("}"),
				Return("results, nil"),
			})),
	})
}

func ExistsMethod(m MessageInfo) Code {
	recv := "r *InMemory" + m.GoName + "Repository"
	return Concat(CodeMonoid, []Code{
		Blank(), Commentf("Exists checks if %s exists", m.GoName),
		Method(recv, "Exists", "ctx context.Context, id string", "(bool, error)",
			Concat(CodeMonoid, []Code{
				If(`id == ""`, Return("false, ErrInvalidID")),
				Blank(),
				Line("r.mu.RLock()"),
				Line("defer r.mu.RUnlock()"),
				Blank(),
				Line("entity, exists := r.data[id]"),
				If("!exists", Return("false, nil")),
				When(m.HasDeletedAt, If("entity.DeletedAt != nil", Return("false, nil"))),
				Return("true, nil"),
			})),
	})
}

func CountMethod(m MessageInfo) Code {
	recv := "r *InMemory" + m.GoName + "Repository"
	return Concat(CodeMonoid, []Code{
		Blank(), Commentf("Count returns total %s (excluding soft-deleted)", m.GoName),
		Method(recv, "Count", "ctx context.Context", "(int64, error)",
			Concat(CodeMonoid, []Code{
				Line("r.mu.RLock()"),
				Line("defer r.mu.RUnlock()"),
				Blank(),
				When(m.HasDeletedAt, Concat(CodeMonoid, []Code{
					Line("count := int64(0)"),
					Line("for _, entity := range r.data {"),
					If("entity.DeletedAt == nil", Line("count++")),
					Line("}"),
					Return("count, nil"),
				})),
				When(!m.HasDeletedAt, Return("int64(len(r.data)), nil")),
			})),
	})
}

func FindByFieldMethod(m MessageInfo, f FieldInfo) Code {
	recv := "r *InMemory" + m.GoName + "Repository"
	methodName := "FindBy" + f.GoName

	if f.IsUnique {
		return Concat(CodeMonoid, []Code{
			Blank(), Commentf("%s finds %s by %s (unique, indexed)", methodName, m.GoName, f.Name),
			Method(recv, methodName, fmt.Sprintf("ctx context.Context, %s %s", lowerFirst(f.GoName), f.GoType), "(*"+m.GoName+", error)",
				Concat(CodeMonoid, []Code{
					Line("r.mu.RLock()"),
					Line("defer r.mu.RUnlock()"),
					Blank(),
					Linef("id, exists := r.idx%s[%s]", f.GoName, lowerFirst(f.GoName)),
					If("!exists", Return("nil, ErrNotFound")),
					Blank(),
					Line("entity, ok := r.data[id]"),
					If("!ok", Return("nil, ErrNotFound")),
					When(m.HasDeletedAt, If("entity.DeletedAt != nil", Return("nil, ErrNotFound"))),
					Return("r.clone(entity), nil"),
				})),
		})
	}

	return Concat(CodeMonoid, []Code{
		Blank(), Commentf("%s finds all %s by %s (scan)", methodName, m.GoName, f.Name),
		Method(recv, methodName, fmt.Sprintf("ctx context.Context, %s %s, limit int", lowerFirst(f.GoName), f.GoType), "([]*"+m.GoName+", error)",
			Concat(CodeMonoid, []Code{
				Line("r.mu.RLock()"),
				Line("defer r.mu.RUnlock()"),
				Blank(),
				Linef("var results []*%s", m.GoName),
				Line("for _, entity := range r.data {"),
				When(m.HasDeletedAt, If("entity.DeletedAt != nil", Line("continue"))),
				Linef("\tif entity.%s == %s {", f.GoName, lowerFirst(f.GoName)),
				Line("\t\tresults = append(results, r.clone(entity))"),
				If("limit > 0 && len(results) >= limit", Line("break")),
				Line("\t}"),
				Line("}"),
				Return("results, nil"),
			})),
	})
}

func FindMethods(m MessageInfo) Code {
	indexed := Filter(m.Fields, func(f FieldInfo) bool { return (f.IsIndexed || f.IsUnique) && !f.IsID })
	return FoldMap(indexed, CodeMonoid, func(f FieldInfo) Code { return FindByFieldMethod(m, f) })
}

func FilterMethod(m MessageInfo) Code {
	recv := "r *InMemory" + m.GoName + "Repository"
	return Concat(CodeMonoid, []Code{
		Blank(), Commentf("Filter finds all %s matching predicate", m.GoName),
		Method(recv, "Filter", "ctx context.Context, predicate func(*"+m.GoName+") bool, limit int", "([]*"+m.GoName+", error)",
			Concat(CodeMonoid, []Code{
				Line("r.mu.RLock()"),
				Line("defer r.mu.RUnlock()"),
				Blank(),
				Linef("var results []*%s", m.GoName),
				Line("for _, entity := range r.data {"),
				When(m.HasDeletedAt, If("entity.DeletedAt != nil", Line("continue"))),
				If("predicate(entity)",
					Concat(CodeMonoid, []Code{
						Line("results = append(results, r.clone(entity))"),
						If("limit > 0 && len(results) >= limit", Line("break")),
					})),
				Line("}"),
				Return("results, nil"),
			})),
		Blank(), Commentf("FindOne finds first %s matching predicate", m.GoName),
		Method(recv, "FindOne", "ctx context.Context, predicate func(*"+m.GoName+") bool", "(*"+m.GoName+", error)",
			Concat(CodeMonoid, []Code{
				Line("results, err := r.Filter(ctx, predicate, 1)"),
				If("err != nil", Return("nil, err")),
				If("len(results) == 0", Return("nil, ErrNotFound")),
				Return("results[0], nil"),
			})),
	})
}

func ClearMethod(m MessageInfo) Code {
	recv := "r *InMemory" + m.GoName + "Repository"
	uniqueFields := Filter(m.Fields, func(f FieldInfo) bool { return f.IsUnique && !f.IsID })

	clearIndexes := FoldMap(uniqueFields, CodeMonoid, func(f FieldInfo) Code {
		return Linef("r.idx%s = make(map[%s]string)", f.GoName, f.GoType)
	})

	return Concat(CodeMonoid, []Code{
		Blank(), Comment("Clear removes all data (useful for tests)"),
		Method(recv, "Clear", "", "",
			Concat(CodeMonoid, []Code{
				Line("r.mu.Lock()"),
				Line("defer r.mu.Unlock()"),
				Blank(),
				Linef("r.data = make(map[string]*%s)", m.GoName),
				When(len(uniqueFields) > 0, clearIndexes),
			})),
	})
}

func SnapshotMethods(m MessageInfo) Code {
	recv := "r *InMemory" + m.GoName + "Repository"
	return Concat(CodeMonoid, []Code{
		Blank(), Comment("Snapshot returns a copy of all data (for debugging/testing)"),
		Method(recv, "Snapshot", "", "map[string]*"+m.GoName,
			Concat(CodeMonoid, []Code{
				Line("r.mu.RLock()"),
				Line("defer r.mu.RUnlock()"),
				Blank(),
				Linef("snapshot := make(map[string]*%s, len(r.data))", m.GoName),
				Line("for id, entity := range r.data {"),
				Line("\tsnapshot[id] = r.clone(entity)"),
				Line("}"),
				Return("snapshot"),
			})),
		Blank(), Comment("Load replaces all data from a snapshot (for testing)"),
		Method(recv, "Load", "data map[string]*"+m.GoName, "",
			Concat(CodeMonoid, []Code{
				Line("r.mu.Lock()"),
				Line("defer r.mu.Unlock()"),
				Blank(),
				Line("r.Clear()"),
				Line("for id, entity := range data {"),
				Line("\tr.data[id] = r.clone(entity)"),
				Line("}"),
			})),
	})
}

// =============================================================================
// MAIN COMPOSITION - FoldMap over messages!
// =============================================================================

func MessageRepository(m MessageInfo) Code {
	return Concat(CodeMonoid, []Code{
		Blank(),
		Linef("// ============================================================================"),
		Linef("// %s Repository - Thread-Safe In-Memory CRUD", m.GoName),
		Linef("// ============================================================================"),
		RepositoryStruct(m), Constructor(m), CloneMethod(m),
		CreateMethod(m), GetMethod(m), UpdateMethod(m), DeleteMethod(m),
		SoftDeleteMethods(m), ListMethod(m), ExistsMethod(m), CountMethod(m),
		FindMethods(m), FilterMethod(m), ClearMethod(m), SnapshotMethods(m),
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
		Imports("context", "errors", "fmt", "sync", "",
			"github.com/google/uuid",
			"google.golang.org/protobuf/proto",
			"google.golang.org/protobuf/types/known/timestamppb"),
		FoldMap(messages, CodeMonoid, MessageRepository),
	})
}

func main() {
	protogen.Options{}.Run(func(gen *protogen.Plugin) error {
		gen.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)
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

			g := gen.NewGeneratedFile(f.GeneratedFilenamePrefix+"_inmemory.pb.go", f.GoImportPath)
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

func zeroValue(goType string) string {
	switch goType {
	case "string":
		return `""`
	case "int32", "int64", "uint32", "uint64", "float32", "float64":
		return "0"
	case "bool":
		return "false"
	default:
		return "nil"
	}
}
