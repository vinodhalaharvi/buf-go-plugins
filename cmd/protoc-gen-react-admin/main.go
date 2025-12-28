// protoc-gen-react-admin generates React admin UI components
// Uses Category Theory: Monoid + Functor + Fold
// Generates: Forms, Tables, Pages, Hooks, Routes
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

// =============================================================================
// CODE PRIMITIVES
// =============================================================================

func Line(s string) Code                            { return Code{Run: func() string { return s + "\n" }} }
func Linef(format string, args ...interface{}) Code { return Line(fmt.Sprintf(format, args...)) }
func Blank() Code                                   { return Line("") }
func Raw(s string) Code                             { return Code{Run: func() string { return s }} }

// =============================================================================
// MESSAGE/FIELD INFO
// =============================================================================

type MessageInfo struct {
	Name, GoName                                    string
	Fields                                          []FieldInfo
	HasID, HasCreatedAt, HasUpdatedAt, HasDeletedAt bool
}

type FieldInfo struct {
	Name, GoName, TsType, InputType string
	IsID, IsReadonly, IsRequired    bool
	IsEnum                          bool
	EnumName                        string
	EnumValues                      []string
}

func ExtractMessageInfo(msg *protogen.Message) MessageInfo {
	fields := Map(msg.Fields, func(f *protogen.Field) FieldInfo { return ExtractFieldInfo(f) })

	hasID, hasCreatedAt, hasUpdatedAt, hasDeletedAt := false, false, false, false
	for _, f := range fields {
		snake := toSnakeCase(f.Name)
		switch {
		case strings.EqualFold(f.Name, "id"):
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
		Fields:       fields,
		HasID:        hasID,
		HasCreatedAt: hasCreatedAt,
		HasUpdatedAt: hasUpdatedAt,
		HasDeletedAt: hasDeletedAt,
	}
}

func ExtractFieldInfo(field *protogen.Field) FieldInfo {
	name := string(field.Desc.Name())
	tsType, inputType := fieldTypes(field)
	isTimestamp := field.Desc.Kind() == protoreflect.MessageKind &&
		field.Message.GoIdent.GoName == "Timestamp"

	info := FieldInfo{
		Name:       name,
		GoName:     field.GoName,
		TsType:     tsType,
		InputType:  inputType,
		IsID:       strings.EqualFold(name, "id"),
		IsReadonly: isTimestamp || strings.EqualFold(name, "id"),
		IsRequired: strings.EqualFold(name, "email") || strings.EqualFold(name, "name"),
		IsEnum:     field.Desc.Kind() == protoreflect.EnumKind,
	}

	if info.IsEnum {
		info.EnumName = field.Enum.GoIdent.GoName
		for _, v := range field.Enum.Values {
			info.EnumValues = append(info.EnumValues, string(v.Desc.Name()))
		}
	}

	return info
}

func fieldTypes(field *protogen.Field) (tsType string, inputType string) {
	name := strings.ToLower(string(field.Desc.Name()))

	switch field.Desc.Kind() {
	case protoreflect.BoolKind:
		return "boolean", "checkbox"
	case protoreflect.Int32Kind, protoreflect.Int64Kind,
		protoreflect.Uint32Kind, protoreflect.Uint64Kind,
		protoreflect.FloatKind, protoreflect.DoubleKind:
		return "number", "number"
	case protoreflect.StringKind:
		if strings.Contains(name, "email") {
			return "string", "email"
		}
		if strings.Contains(name, "password") {
			return "string", "password"
		}
		if strings.Contains(name, "url") || strings.Contains(name, "link") {
			return "string", "url"
		}
		if strings.Contains(name, "phone") {
			return "string", "tel"
		}
		if strings.Contains(name, "description") || strings.Contains(name, "content") || strings.Contains(name, "bio") {
			return "string", "textarea"
		}
		return "string", "text"
	case protoreflect.EnumKind:
		return field.Enum.GoIdent.GoName, "select"
	case protoreflect.MessageKind:
		if field.Message.GoIdent.GoName == "Timestamp" {
			return "Date", "datetime-local"
		}
		return "object", "text"
	default:
		return "unknown", "text"
	}
}

// =============================================================================
// TYPESCRIPT/REACT GENERATORS
// =============================================================================

func FileHeader() Code {
	return Concat(CodeMonoid, []Code{
		Line("// Code generated by protoc-gen-react-admin. DO NOT EDIT."),
		Line("// Generated using Category Theory: Monoid + Functor + Fold"),
		Blank(),
	})
}

// =============================================================================
// HOOKS GENERATOR
// =============================================================================

func GenerateHooks(messages []MessageInfo, pkgName string) Code {
	return Concat(CodeMonoid, []Code{
		FileHeader(),
		Line(`import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";`),
		Line(`import { createClient } from "@connectrpc/connect";`),
		Line(`import { createConnectTransport } from "@connectrpc/connect-web";`),
		Linef(`import { %sService } from "./gen/%s/service_connect";`, messages[0].GoName, pkgName),
		Linef(`import type { %s } from "./gen/%s/models_pb";`, strings.Join(Map(messages, func(m MessageInfo) string { return m.GoName }), ", "), pkgName),
		Blank(),
		Line(`const transport = createConnectTransport({`),
		Line(`  baseUrl: import.meta.env.VITE_API_URL || "http://localhost:8080",`),
		Line(`});`),
		Blank(),
		FoldMap(messages, CodeMonoid, GenerateEntityHooks),
	})
}

func GenerateEntityHooks(m MessageInfo) Code {
	lower := lowerFirst(m.GoName)
	return Concat(CodeMonoid, []Code{
		Line(`// =============================================================================`),
		Linef(`// %s Hooks`, m.GoName),
		Line(`// =============================================================================`),
		Blank(),
		Linef(`const %sClient = createClient(%sService, transport);`, lower, m.GoName),
		Blank(),
		// List hook
		Linef(`export function use%ss(pageSize = 20) {`, m.GoName),
		Line(`  return useQuery({`),
		Linef(`    queryKey: ["%ss"],`, lower),
		Linef(`    queryFn: () => %sClient.list%ss({ pageSize }),`, lower, m.GoName),
		Line(`  });`),
		Line(`}`),
		Blank(),
		// Get hook
		Linef(`export function use%s(id: string | undefined) {`, m.GoName),
		Line(`  return useQuery({`),
		Linef(`    queryKey: ["%s", id],`, lower),
		Linef(`    queryFn: () => %sClient.get%s({ id: id! }),`, lower, m.GoName),
		Line(`    enabled: !!id,`),
		Line(`  });`),
		Line(`}`),
		Blank(),
		// Create hook
		Linef(`export function useCreate%s() {`, m.GoName),
		Line(`  const queryClient = useQueryClient();`),
		Line(`  return useMutation({`),
		Linef(`    mutationFn: (%s: Partial<%s>) => %sClient.create%s({ %s }),`, lower, m.GoName, lower, m.GoName, lower),
		Linef(`    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["%ss"] }),`, lower),
		Line(`  });`),
		Line(`}`),
		Blank(),
		// Update hook
		Linef(`export function useUpdate%s() {`, m.GoName),
		Line(`  const queryClient = useQueryClient();`),
		Line(`  return useMutation({`),
		Linef(`    mutationFn: (%s: %s) => %sClient.update%s({ %s }),`, lower, m.GoName, lower, m.GoName, lower),
		Line(`    onSuccess: (_, vars) => {`),
		Linef(`      queryClient.invalidateQueries({ queryKey: ["%ss"] });`, lower),
		Linef(`      queryClient.invalidateQueries({ queryKey: ["%s", vars.id] });`, lower),
		Line(`    },`),
		Line(`  });`),
		Line(`}`),
		Blank(),
		// Delete hook
		Linef(`export function useDelete%s() {`, m.GoName),
		Line(`  const queryClient = useQueryClient();`),
		Line(`  return useMutation({`),
		Linef(`    mutationFn: (id: string) => %sClient.delete%s({ id }),`, lower, m.GoName),
		Linef(`    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["%ss"] }),`, lower),
		Line(`  });`),
		Line(`}`),
		Blank(),
	})
}

// =============================================================================
// FORM GENERATOR
// =============================================================================

func GenerateForm(m MessageInfo) Code {
	editableFields := Filter(m.Fields, func(f FieldInfo) bool {
		return !f.IsReadonly && !f.IsID
	})

	return Concat(CodeMonoid, []Code{
		FileHeader(),
		Line(`import { useState } from "react";`),
		Linef(`import type { %s } from "../types";`, m.GoName),
		When(hasEnums(m), GenerateEnumImports(m)),
		Blank(),
		Linef(`interface %sFormProps {`, m.GoName),
		Linef(`  initial?: Partial<%s>;`, m.GoName),
		Linef(`  onSubmit: (data: Partial<%s>) => void;`, m.GoName),
		Line(`  onCancel?: () => void;`),
		Line(`  isLoading?: boolean;`),
		Line(`}`),
		Blank(),
		Linef(`export function %sForm({ initial, onSubmit, onCancel, isLoading }: %sFormProps) {`, m.GoName, m.GoName),
		Linef(`  const [form, setForm] = useState<Partial<%s>>(initial ?? {});`, m.GoName),
		Blank(),
		Line(`  const handleSubmit = (e: React.FormEvent) => {`),
		Line(`    e.preventDefault();`),
		Line(`    onSubmit(form);`),
		Line(`  };`),
		Blank(),
		Line(`  const update = <K extends keyof typeof form>(key: K, value: typeof form[K]) => {`),
		Line(`    setForm((prev) => ({ ...prev, [key]: value }));`),
		Line(`  };`),
		Blank(),
		Line(`  return (`),
		Line(`    <form onSubmit={handleSubmit} className="space-y-4">`),
		FoldMap(editableFields, CodeMonoid, GenerateFormField),
		Blank(),
		Line(`      <div className="flex gap-2 pt-4">`),
		Line(`        <button`),
		Line(`          type="submit"`),
		Line(`          disabled={isLoading}`),
		Line(`          className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"`),
		Line(`        >`),
		Line(`          {isLoading ? "Saving..." : "Save"}`),
		Line(`        </button>`),
		Line(`        {onCancel && (`),
		Line(`          <button`),
		Line(`            type="button"`),
		Line(`            onClick={onCancel}`),
		Line(`            className="px-4 py-2 bg-gray-200 rounded hover:bg-gray-300"`),
		Line(`          >`),
		Line(`            Cancel`),
		Line(`          </button>`),
		Line(`        )}`),
		Line(`      </div>`),
		Line(`    </form>`),
		Line(`  );`),
		Line(`}`),
	})
}

func GenerateFormField(f FieldInfo) Code {
	label := toTitleCase(f.Name)
	fieldName := lowerFirst(f.GoName)

	if f.IsEnum {
		return Concat(CodeMonoid, []Code{
			Blank(),
			Line(`      <div>`),
			Linef(`        <label className="block text-sm font-medium text-gray-700">%s</label>`, label),
			Line(`        <select`),
			Linef(`          value={form.%s ?? ""}`, fieldName),
			Linef(`          onChange={(e) => update("%s", e.target.value)}`, fieldName),
			Line(`          className="mt-1 block w-full rounded border-gray-300 shadow-sm focus:border-blue-500 focus:ring-blue-500"`),
			Line(`        >`),
			Line(`          <option value="">Select...</option>`),
			FoldMap(f.EnumValues, CodeMonoid, func(v string) Code {
				return Linef(`          <option value="%s">%s</option>`, v, toTitleCase(strings.TrimPrefix(v, strings.ToUpper(toSnakeCase(f.EnumName))+"_")))
			}),
			Line(`        </select>`),
			Line(`      </div>`),
		})
	}

	if f.InputType == "checkbox" {
		return Concat(CodeMonoid, []Code{
			Blank(),
			Line(`      <div className="flex items-center gap-2">`),
			Line(`        <input`),
			Line(`          type="checkbox"`),
			Linef(`          id="%s"`, fieldName),
			Linef(`          checked={form.%s ?? false}`, fieldName),
			Linef(`          onChange={(e) => update("%s", e.target.checked)}`, fieldName),
			Line(`          className="rounded border-gray-300 text-blue-600 focus:ring-blue-500"`),
			Line(`        />`),
			Linef(`        <label htmlFor="%s" className="text-sm font-medium text-gray-700">%s</label>`, fieldName, label),
			Line(`      </div>`),
		})
	}

	if f.InputType == "textarea" {
		return Concat(CodeMonoid, []Code{
			Blank(),
			Line(`      <div>`),
			Linef(`        <label className="block text-sm font-medium text-gray-700">%s</label>`, label),
			Line(`        <textarea`),
			Linef(`          value={form.%s ?? ""}`, fieldName),
			Linef(`          onChange={(e) => update("%s", e.target.value)}`, fieldName),
			When(f.IsRequired, Line(`          required`)),
			Line(`          rows={4}`),
			Line(`          className="mt-1 block w-full rounded border-gray-300 shadow-sm focus:border-blue-500 focus:ring-blue-500"`),
			Line(`        />`),
			Line(`      </div>`),
		})
	}

	return Concat(CodeMonoid, []Code{
		Blank(),
		Line(`      <div>`),
		Linef(`        <label className="block text-sm font-medium text-gray-700">%s</label>`, label),
		Line(`        <input`),
		Linef(`          type="%s"`, f.InputType),
		Linef(`          value={form.%s ?? ""}`, fieldName),
		Linef(`          onChange={(e) => update("%s", e.target.%s)}`, fieldName, inputValue(f.InputType)),
		When(f.IsRequired, Line(`          required`)),
		Line(`          className="mt-1 block w-full rounded border-gray-300 shadow-sm focus:border-blue-500 focus:ring-blue-500"`),
		Line(`        />`),
		Line(`      </div>`),
	})
}

func inputValue(inputType string) string {
	if inputType == "number" {
		return "valueAsNumber"
	}
	return "value"
}

func hasEnums(m MessageInfo) bool {
	for _, f := range m.Fields {
		if f.IsEnum {
			return true
		}
	}
	return false
}

func GenerateEnumImports(m MessageInfo) Code {
	enums := Filter(m.Fields, func(f FieldInfo) bool { return f.IsEnum })
	enumNames := Map(enums, func(f FieldInfo) string { return f.EnumName })
	return Linef(`import { %s } from "../types";`, strings.Join(enumNames, ", "))
}

// =============================================================================
// TABLE GENERATOR
// =============================================================================

func GenerateTable(m MessageInfo) Code {
	displayFields := Filter(m.Fields, func(f FieldInfo) bool {
		return !strings.Contains(strings.ToLower(f.Name), "deleted")
	})

	return Concat(CodeMonoid, []Code{
		FileHeader(),
		Line(`import { Link } from "react-router-dom";`),
		Linef(`import type { %s } from "../types";`, m.GoName),
		Blank(),
		Linef(`interface %sTableProps {`, m.GoName),
		Linef(`  data: %s[];`, m.GoName),
		Line(`  onDelete?: (id: string) => void;`),
		Line(`  isDeleting?: boolean;`),
		Line(`}`),
		Blank(),
		Linef(`export function %sTable({ data, onDelete, isDeleting }: %sTableProps) {`, m.GoName, m.GoName),
		Line(`  if (data.length === 0) {`),
		Line(`    return (`),
		Line(`      <div className="text-center py-12 text-gray-500">`),
		Linef(`        No %ss found`, lowerFirst(m.GoName)),
		Line(`      </div>`),
		Line(`    );`),
		Line(`  }`),
		Blank(),
		Line(`  return (`),
		Line(`    <div className="overflow-x-auto">`),
		Line(`      <table className="min-w-full divide-y divide-gray-200">`),
		Line(`        <thead className="bg-gray-50">`),
		Line(`          <tr>`),
		FoldMap(displayFields, CodeMonoid, func(f FieldInfo) Code {
			return Linef(`            <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">%s</th>`, toTitleCase(f.Name))
		}),
		Line(`            <th className="px-6 py-3 text-right text-xs font-medium text-gray-500 uppercase tracking-wider">Actions</th>`),
		Line(`          </tr>`),
		Line(`        </thead>`),
		Line(`        <tbody className="bg-white divide-y divide-gray-200">`),
		Line(`          {data.map((item) => (`),
		Line(`            <tr key={item.id} className="hover:bg-gray-50">`),
		FoldMap(displayFields, CodeMonoid, GenerateTableCell),
		Line(`              <td className="px-6 py-4 text-right text-sm font-medium space-x-2">`),
		Linef(`                <Link to={` + "`/${item.id}`" + `} className="text-blue-600 hover:text-blue-900">`),
		Line(`                  View`),
		Line(`                </Link>`),
		Linef(`                <Link to={` + "`/${item.id}/edit`" + `} className="text-green-600 hover:text-green-900">`),
		Line(`                  Edit`),
		Line(`                </Link>`),
		Line(`                {onDelete && (`),
		Line(`                  <button`),
		Line(`                    onClick={() => onDelete(item.id)}`),
		Line(`                    disabled={isDeleting}`),
		Line(`                    className="text-red-600 hover:text-red-900 disabled:opacity-50"`),
		Line(`                  >`),
		Line(`                    Delete`),
		Line(`                  </button>`),
		Line(`                )}`),
		Line(`              </td>`),
		Line(`            </tr>`),
		Line(`          ))}`),
		Line(`        </tbody>`),
		Line(`      </table>`),
		Line(`    </div>`),
		Line(`  );`),
		Line(`}`),
	})
}

func GenerateTableCell(f FieldInfo) Code {
	fieldName := lowerFirst(f.GoName)

	if f.InputType == "checkbox" {
		return Concat(CodeMonoid, []Code{
			Linef(`              <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-900">`),
			Linef(`                {item.%s ? "Yes" : "No"}`, fieldName),
			Line(`              </td>`),
		})
	}

	if f.InputType == "datetime-local" {
		return Concat(CodeMonoid, []Code{
			Line(`              <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">`),
			Linef(`                {item.%s ? new Date(item.%s).toLocaleString() : "-"}`, fieldName, fieldName),
			Line(`              </td>`),
		})
	}

	if f.IsEnum {
		return Concat(CodeMonoid, []Code{
			Line(`              <td className="px-6 py-4 whitespace-nowrap">`),
			Linef(`                <span className="px-2 py-1 text-xs rounded-full bg-gray-100">{item.%s}</span>`, fieldName),
			Line(`              </td>`),
		})
	}

	return Concat(CodeMonoid, []Code{
		Line(`              <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-900">`),
		Linef(`                {item.%s ?? "-"}`, fieldName),
		Line(`              </td>`),
	})
}

// =============================================================================
// PAGES GENERATOR
// =============================================================================

func GenerateListPage(m MessageInfo) Code {
	lower := lowerFirst(m.GoName)
	return Concat(CodeMonoid, []Code{
		FileHeader(),
		Line(`import { Link } from "react-router-dom";`),
		Linef(`import { use%ss, useDelete%s } from "../hooks";`, m.GoName, m.GoName),
		Linef(`import { %sTable } from "../components/%sTable";`, m.GoName, m.GoName),
		Blank(),
		Linef(`export function %sListPage() {`, m.GoName),
		Linef(`  const { data, isLoading, error } = use%ss();`, m.GoName),
		Linef(`  const deleteMutation = useDelete%s();`, m.GoName),
		Blank(),
		Line(`  if (isLoading) {`),
		Line(`    return <div className="p-8 text-center">Loading...</div>;`),
		Line(`  }`),
		Blank(),
		Line(`  if (error) {`),
		Line(`    return <div className="p-8 text-center text-red-600">Error: {error.message}</div>;`),
		Line(`  }`),
		Blank(),
		Line(`  return (`),
		Line(`    <div className="p-8">`),
		Line(`      <div className="flex justify-between items-center mb-6">`),
		Linef(`        <h1 className="text-2xl font-bold">%ss</h1>`, m.GoName),
		Line(`        <Link`),
		Line(`          to="/new"`),
		Line(`          className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700"`),
		Line(`        >`),
		Linef(`          New %s`, m.GoName),
		Line(`        </Link>`),
		Line(`      </div>`),
		Blank(),
		Linef(`      <%sTable`, m.GoName),
		Linef(`        data={data?.%ss ?? []}`, lower),
		Line(`        onDelete={(id) => deleteMutation.mutate(id)}`),
		Line(`        isDeleting={deleteMutation.isPending}`),
		Line(`      />`),
		Line(`    </div>`),
		Line(`  );`),
		Line(`}`),
	})
}

func GenerateDetailPage(m MessageInfo) Code {
	lower := lowerFirst(m.GoName)
	displayFields := Filter(m.Fields, func(f FieldInfo) bool {
		return !strings.Contains(strings.ToLower(f.Name), "deleted")
	})

	return Concat(CodeMonoid, []Code{
		FileHeader(),
		Line(`import { useParams, Link, useNavigate } from "react-router-dom";`),
		Linef(`import { use%s, useDelete%s } from "../hooks";`, m.GoName, m.GoName),
		Blank(),
		Linef(`export function %sDetailPage() {`, m.GoName),
		Line(`  const { id } = useParams<{ id: string }>();`),
		Line(`  const navigate = useNavigate();`),
		Linef(`  const { data, isLoading, error } = use%s(id);`, m.GoName),
		Linef(`  const deleteMutation = useDelete%s();`, m.GoName),
		Blank(),
		Line(`  const handleDelete = async () => {`),
		Line(`    if (!id || !confirm("Are you sure?")) return;`),
		Line(`    await deleteMutation.mutateAsync(id);`),
		Line(`    navigate("/");`),
		Line(`  };`),
		Blank(),
		Line(`  if (isLoading) return <div className="p-8 text-center">Loading...</div>;`),
		Line(`  if (error) return <div className="p-8 text-center text-red-600">Error: {error.message}</div>;`),
		Linef(`  if (!data?.%s) return <div className="p-8 text-center">Not found</div>;`, lower),
		Blank(),
		Linef(`  const %s = data.%s;`, lower, lower),
		Blank(),
		Line(`  return (`),
		Line(`    <div className="p-8 max-w-2xl">`),
		Line(`      <div className="flex justify-between items-center mb-6">`),
		Linef(`        <h1 className="text-2xl font-bold">%s Details</h1>`, m.GoName),
		Line(`        <div className="space-x-2">`),
		Line(`          <Link`),
		Line("            to={`/${id}/edit`}"),
		Line(`            className="px-4 py-2 bg-green-600 text-white rounded hover:bg-green-700"`),
		Line(`          >`),
		Line(`            Edit`),
		Line(`          </Link>`),
		Line(`          <button`),
		Line(`            onClick={handleDelete}`),
		Line(`            disabled={deleteMutation.isPending}`),
		Line(`            className="px-4 py-2 bg-red-600 text-white rounded hover:bg-red-700 disabled:opacity-50"`),
		Line(`          >`),
		Line(`            Delete`),
		Line(`          </button>`),
		Line(`        </div>`),
		Line(`      </div>`),
		Blank(),
		Line(`      <dl className="divide-y divide-gray-200">`),
		FoldMap(displayFields, CodeMonoid, func(f FieldInfo) Code {
			return GenerateDetailField(f, lower)
		}),
		Line(`      </dl>`),
		Blank(),
		Line(`      <div className="mt-6">`),
		Line(`        <Link to="/" className="text-blue-600 hover:text-blue-800">← Back to list</Link>`),
		Line(`      </div>`),
		Line(`    </div>`),
		Line(`  );`),
		Line(`}`),
	})
}

func GenerateDetailField(f FieldInfo, entityVar string) Code {
	fieldName := lowerFirst(f.GoName)
	label := toTitleCase(f.Name)

	value := fmt.Sprintf(`{%s.%s ?? "-"}`, entityVar, fieldName)
	if f.InputType == "datetime-local" {
		value = fmt.Sprintf(`{%s.%s ? new Date(%s.%s).toLocaleString() : "-"}`, entityVar, fieldName, entityVar, fieldName)
	} else if f.InputType == "checkbox" {
		value = fmt.Sprintf(`{%s.%s ? "Yes" : "No"}`, entityVar, fieldName)
	}

	return Concat(CodeMonoid, []Code{
		Line(`        <div className="py-3 grid grid-cols-3 gap-4">`),
		Linef(`          <dt className="text-sm font-medium text-gray-500">%s</dt>`, label),
		Linef(`          <dd className="text-sm text-gray-900 col-span-2">%s</dd>`, value),
		Line(`        </div>`),
	})
}

func GenerateCreatePage(m MessageInfo) Code {
	lower := lowerFirst(m.GoName)
	return Concat(CodeMonoid, []Code{
		FileHeader(),
		Line(`import { useNavigate } from "react-router-dom";`),
		Linef(`import { useCreate%s } from "../hooks";`, m.GoName),
		Linef(`import { %sForm } from "../components/%sForm";`, m.GoName, m.GoName),
		Blank(),
		Linef(`export function %sCreatePage() {`, m.GoName),
		Line(`  const navigate = useNavigate();`),
		Linef(`  const mutation = useCreate%s();`, m.GoName),
		Blank(),
		Linef(`  const handleSubmit = async (%s: any) => {`, lower),
		Linef(`    const result = await mutation.mutateAsync(%s);`, lower),
		Linef(`    navigate(`+"`/${result.%s?.id}`"+`);`, lower),
		Line(`  };`),
		Blank(),
		Line(`  return (`),
		Line(`    <div className="p-8 max-w-2xl">`),
		Linef(`      <h1 className="text-2xl font-bold mb-6">New %s</h1>`, m.GoName),
		Blank(),
		Linef(`      <%sForm`, m.GoName),
		Line(`        onSubmit={handleSubmit}`),
		Line(`        onCancel={() => navigate("/")}`),
		Line(`        isLoading={mutation.isPending}`),
		Line(`      />`),
		Blank(),
		Line(`      {mutation.error && (`),
		Line(`        <div className="mt-4 p-4 bg-red-50 text-red-600 rounded">`),
		Line(`          {mutation.error.message}`),
		Line(`        </div>`),
		Line(`      )}`),
		Line(`    </div>`),
		Line(`  );`),
		Line(`}`),
	})
}

func GenerateEditPage(m MessageInfo) Code {
	lower := lowerFirst(m.GoName)
	return Concat(CodeMonoid, []Code{
		FileHeader(),
		Line(`import { useParams, useNavigate } from "react-router-dom";`),
		Linef(`import { use%s, useUpdate%s } from "../hooks";`, m.GoName, m.GoName),
		Linef(`import { %sForm } from "../components/%sForm";`, m.GoName, m.GoName),
		Blank(),
		Linef(`export function %sEditPage() {`, m.GoName),
		Line(`  const { id } = useParams<{ id: string }>();`),
		Line(`  const navigate = useNavigate();`),
		Linef(`  const { data, isLoading } = use%s(id);`, m.GoName),
		Linef(`  const mutation = useUpdate%s();`, m.GoName),
		Blank(),
		Linef(`  const handleSubmit = async (%s: any) => {`, lower),
		Linef(`    await mutation.mutateAsync({ ...%s, id });`, lower),
		Line("    navigate(`/${id}`);"),
		Line(`  };`),
		Blank(),
		Line(`  if (isLoading) return <div className="p-8 text-center">Loading...</div>;`),
		Blank(),
		Line(`  return (`),
		Line(`    <div className="p-8 max-w-2xl">`),
		Linef(`      <h1 className="text-2xl font-bold mb-6">Edit %s</h1>`, m.GoName),
		Blank(),
		Linef(`      <%sForm`, m.GoName),
		Linef(`        initial={data?.%s}`, lower),
		Line(`        onSubmit={handleSubmit}`),
		Line("        onCancel={() => navigate(`/${id}`)}"),
		Line(`        isLoading={mutation.isPending}`),
		Line(`      />`),
		Blank(),
		Line(`      {mutation.error && (`),
		Line(`        <div className="mt-4 p-4 bg-red-50 text-red-600 rounded">`),
		Line(`          {mutation.error.message}`),
		Line(`        </div>`),
		Line(`      )}`),
		Line(`    </div>`),
		Line(`  );`),
		Line(`}`),
	})
}

// =============================================================================
// ROUTER GENERATOR
// =============================================================================

func GenerateRouter(messages []MessageInfo) Code {
	return Concat(CodeMonoid, []Code{
		FileHeader(),
		Line(`import { createBrowserRouter, RouterProvider, Outlet, Link } from "react-router-dom";`),
		Line(`import { QueryClient, QueryClientProvider } from "@tanstack/react-query";`),
		Blank(),
		Line(`// Import pages`),
		FoldMap(messages, CodeMonoid, func(m MessageInfo) Code {
			return Concat(CodeMonoid, []Code{
				Linef(`import { %sListPage } from "./pages/%sListPage";`, m.GoName, m.GoName),
				Linef(`import { %sDetailPage } from "./pages/%sDetailPage";`, m.GoName, m.GoName),
				Linef(`import { %sCreatePage } from "./pages/%sCreatePage";`, m.GoName, m.GoName),
				Linef(`import { %sEditPage } from "./pages/%sEditPage";`, m.GoName, m.GoName),
			})
		}),
		Blank(),
		Line(`const queryClient = new QueryClient();`),
		Blank(),
		Line(`function Layout() {`),
		Line(`  return (`),
		Line(`    <div className="min-h-screen bg-gray-100">`),
		Line(`      <nav className="bg-white shadow">`),
		Line(`        <div className="max-w-7xl mx-auto px-4 py-3">`),
		Line(`          <div className="flex space-x-8">`),
		FoldMap(messages, CodeMonoid, func(m MessageInfo) Code {
			return Linef(`            <Link to="/%ss" className="text-gray-700 hover:text-gray-900">%ss</Link>`, lowerFirst(m.GoName), m.GoName)
		}),
		Line(`          </div>`),
		Line(`        </div>`),
		Line(`      </nav>`),
		Line(`      <main className="max-w-7xl mx-auto">`),
		Line(`        <Outlet />`),
		Line(`      </main>`),
		Line(`    </div>`),
		Line(`  );`),
		Line(`}`),
		Blank(),
		Line(`const router = createBrowserRouter([`),
		Line(`  {`),
		Line(`    path: "/",`),
		Line(`    element: <Layout />,`),
		Line(`    children: [`),
		FoldMap(messages, CodeMonoid, GenerateEntityRoutes),
		Line(`    ],`),
		Line(`  },`),
		Line(`]);`),
		Blank(),
		Line(`export function App() {`),
		Line(`  return (`),
		Line(`    <QueryClientProvider client={queryClient}>`),
		Line(`      <RouterProvider router={router} />`),
		Line(`    </QueryClientProvider>`),
		Line(`  );`),
		Line(`}`),
	})
}

func GenerateEntityRoutes(m MessageInfo) Code {
	lower := lowerFirst(m.GoName)
	return Concat(CodeMonoid, []Code{
		Linef(`      { path: "%ss", element: <%sListPage /> },`, lower, m.GoName),
		Linef(`      { path: "%ss/new", element: <%sCreatePage /> },`, lower, m.GoName),
		Linef(`      { path: "%ss/:id", element: <%sDetailPage /> },`, lower, m.GoName),
		Linef(`      { path: "%ss/:id/edit", element: <%sEditPage /> },`, lower, m.GoName),
	})
}

// =============================================================================
// MAIN - Generate all files
// =============================================================================

func main() {
	protogen.Options{}.Run(func(gen *protogen.Plugin) error {
		// Track directories where we've generated config files
		configGenerated := make(map[string]bool)

		for _, f := range gen.Files {
			if !f.Generate || len(f.Messages) == 0 {
				continue
			}

			messages := Filter(
				Map(f.Messages, ExtractMessageInfo),
				func(m MessageInfo) bool { return m.HasID },
			)

			if len(messages) == 0 {
				continue
			}

			pkgName := string(f.GoPackageName)
			basePath := f.GeneratedFilenamePrefix

			// Generate hooks
			hooksFile := gen.NewGeneratedFile(basePath+"_hooks.ts", f.GoImportPath)
			hooksFile.P(GenerateHooks(messages, pkgName).Run())

			// Generate router/App
			routerFile := gen.NewGeneratedFile(basePath+"_app.tsx", f.GoImportPath)
			routerFile.P(GenerateRouter(messages).Run())

			// Generate components and pages for each message
			for _, m := range messages {
				// Form component
				formFile := gen.NewGeneratedFile(basePath+"_"+lowerFirst(m.GoName)+"_form.tsx", f.GoImportPath)
				formFile.P(GenerateForm(m).Run())

				// Table component
				tableFile := gen.NewGeneratedFile(basePath+"_"+lowerFirst(m.GoName)+"_table.tsx", f.GoImportPath)
				tableFile.P(GenerateTable(m).Run())

				// Pages
				listPage := gen.NewGeneratedFile(basePath+"_"+lowerFirst(m.GoName)+"_list_page.tsx", f.GoImportPath)
				listPage.P(GenerateListPage(m).Run())

				detailPage := gen.NewGeneratedFile(basePath+"_"+lowerFirst(m.GoName)+"_detail_page.tsx", f.GoImportPath)
				detailPage.P(GenerateDetailPage(m).Run())

				createPage := gen.NewGeneratedFile(basePath+"_"+lowerFirst(m.GoName)+"_create_page.tsx", f.GoImportPath)
				createPage.P(GenerateCreatePage(m).Run())

				editPage := gen.NewGeneratedFile(basePath+"_"+lowerFirst(m.GoName)+"_edit_page.tsx", f.GoImportPath)
				editPage.P(GenerateEditPage(m).Run())
			}

			// Generate config files (once per directory)
			dir := basePath[:strings.LastIndex(basePath, "/")]
			if !configGenerated[dir] {
				genConfigFiles(gen, basePath)
				configGenerated[dir] = true
			}
		}
		return nil
	})
}

// =============================================================================
// CONFIG FILE GENERATORS
// =============================================================================

func genConfigFiles(gen *protogen.Plugin, basePath string) {
	// Extract directory from basePath
	dir := basePath[:strings.LastIndex(basePath, "/")]

	// tsconfig.json
	tsconfig := gen.NewGeneratedFile(dir+"/tsconfig.json", "")
	tsconfig.P(GenerateTsConfig().Run())

	// tsconfig.node.json
	tsconfigNode := gen.NewGeneratedFile(dir+"/tsconfig.node.json", "")
	tsconfigNode.P(GenerateTsConfigNode().Run())

	// package.json
	packageJson := gen.NewGeneratedFile(dir+"/package.json", "")
	packageJson.P(GeneratePackageJson().Run())

	// vite.config.ts
	viteConfig := gen.NewGeneratedFile(dir+"/vite.config.ts", "")
	viteConfig.P(GenerateViteConfig().Run())

	// tailwind.config.js
	tailwindConfig := gen.NewGeneratedFile(dir+"/tailwind.config.js", "")
	tailwindConfig.P(GenerateTailwindConfig().Run())

	// postcss.config.js
	postcssConfig := gen.NewGeneratedFile(dir+"/postcss.config.js", "")
	postcssConfig.P(GeneratePostcssConfig().Run())

	// index.html
	indexHtml := gen.NewGeneratedFile(dir+"/index.html", "")
	indexHtml.P(GenerateIndexHtml().Run())

	// index.css
	indexCss := gen.NewGeneratedFile(dir+"/index.css", "")
	indexCss.P(GenerateIndexCss().Run())

	// main.tsx
	mainTsx := gen.NewGeneratedFile(dir+"/main.tsx", "")
	mainTsx.P(GenerateMainTsx().Run())
}

func GenerateTsConfig() Code {
	return Raw(`{
  "compilerOptions": {
    "target": "ES2020",
    "useDefineForClassFields": true,
    "lib": ["ES2020", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "skipLibCheck": true,

    /* Bundler mode */
    "moduleResolution": "bundler",
    "allowImportingTsExtensions": true,
    "resolveJsonModule": true,
    "isolatedModules": true,
    "noEmit": true,
    "jsx": "react-jsx",

    /* Linting */
    "strict": true,
    "noUnusedLocals": false,
    "noUnusedParameters": false,
    "noFallthroughCasesInSwitch": true
  },
  "include": ["**/*.ts", "**/*.tsx"],
  "references": [{ "path": "./tsconfig.node.json" }]
}
`)
}

func GenerateTsConfigNode() Code {
	return Raw(`{
  "compilerOptions": {
    "composite": true,
    "skipLibCheck": true,
    "module": "ESNext",
    "moduleResolution": "bundler",
    "allowSyntheticDefaultImports": true,
    "strict": true
  },
  "include": ["vite.config.ts"]
}
`)
}

func GeneratePackageJson() Code {
	return Raw(`{
  "name": "generated-admin-ui",
  "private": true,
  "version": "0.0.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc && vite build",
    "lint": "eslint . --ext ts,tsx --report-unused-disable-directives --max-warnings 0",
    "preview": "vite preview"
  },
  "dependencies": {
    "@connectrpc/connect": "^1.4.0",
    "@connectrpc/connect-web": "^1.4.0",
    "@tanstack/react-query": "^5.17.0",
    "react": "^18.2.0",
    "react-dom": "^18.2.0",
    "react-router-dom": "^6.21.0"
  },
  "devDependencies": {
    "@types/react": "^18.2.43",
    "@types/react-dom": "^18.2.17",
    "@typescript-eslint/eslint-plugin": "^6.14.0",
    "@typescript-eslint/parser": "^6.14.0",
    "@vitejs/plugin-react": "^4.2.1",
    "autoprefixer": "^10.4.16",
    "eslint": "^8.55.0",
    "eslint-plugin-react-hooks": "^4.6.0",
    "eslint-plugin-react-refresh": "^0.4.5",
    "postcss": "^8.4.32",
    "tailwindcss": "^3.4.0",
    "typescript": "^5.2.2",
    "vite": "^5.0.8"
  }
}
`)
}

func GenerateViteConfig() Code {
	return Raw(`import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    port: 3000,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/api/, ''),
      },
    },
  },
})
`)
}

func GenerateTailwindConfig() Code {
	return Raw(`/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {},
  },
  plugins: [],
}
`)
}

func GeneratePostcssConfig() Code {
	return Raw(`export default {
  plugins: {
    tailwindcss: {},
    autoprefixer: {},
  },
}
`)
}

func GenerateIndexHtml() Code {
	return Raw(`<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <link rel="icon" type="image/svg+xml" href="/vite.svg" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Admin UI</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/main.tsx"></script>
  </body>
</html>
`)
}

func GenerateIndexCss() Code {
	return Raw(`@tailwind base;
@tailwind components;
@tailwind utilities;
`)
}

func GenerateMainTsx() Code {
	return Raw(`import React from 'react'
import ReactDOM from 'react-dom/client'
import { App } from './App'
import './index.css'

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)
`)
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

func toTitleCase(s string) string {
	s = strings.ReplaceAll(toSnakeCase(s), "_", " ")
	words := strings.Fields(s)
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[:1]) + word[1:]
		}
	}
	return strings.Join(words, " ")
}
