// protoc-gen-mock generates fake data generators and seeders
// Uses field names/types to infer appropriate fake data
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

func Line(s string) Code                            { return Code{Run: func() string { return s + "\n" }} }
func Linef(format string, args ...interface{}) Code { return Line(fmt.Sprintf(format, args...)) }
func Blank() Code                                   { return Line("") }

// =============================================================================
// MESSAGE INFO
// =============================================================================

type MessageInfo struct {
	Name   string
	Fields []FieldInfo
}

type FieldInfo struct {
	Name       string
	GoName     string
	ProtoKind  protoreflect.Kind
	IsRepeated bool
	IsMap      bool
	IsEnum     bool
	EnumName   string
	EnumValues []string
	MessageRef string // for nested messages
}

func ExtractMessageInfo(msg *protogen.Message) MessageInfo {
	fields := Map(msg.Fields, func(f *protogen.Field) FieldInfo {
		fi := FieldInfo{
			Name:       string(f.Desc.Name()),
			GoName:     f.GoName,
			ProtoKind:  f.Desc.Kind(),
			IsRepeated: f.Desc.IsList(),
			IsMap:      f.Desc.IsMap(),
		}

		if f.Desc.Kind() == protoreflect.EnumKind && f.Enum != nil {
			fi.IsEnum = true
			fi.EnumName = f.Enum.GoIdent.GoName
			for _, v := range f.Enum.Values {
				fi.EnumValues = append(fi.EnumValues, string(v.Desc.Name()))
			}
		}

		if f.Desc.Kind() == protoreflect.MessageKind && f.Message != nil {
			fi.MessageRef = f.Message.GoIdent.GoName
		}

		return fi
	})

	return MessageInfo{
		Name:   msg.GoIdent.GoName,
		Fields: fields,
	}
}

func hasIDField(msg *protogen.Message) bool {
	for _, f := range msg.Fields {
		if strings.EqualFold(string(f.Desc.Name()), "id") {
			return true
		}
	}
	return false
}

// =============================================================================
// FAKER INFERENCE (Field Name/Type → Faker Function)
// =============================================================================

func InferFaker(f FieldInfo) string {
	name := strings.ToLower(f.Name)

	// Handle special message types
	if f.MessageRef == "Timestamp" {
		return "timestamppb.New(gofakeit.Date())"
	}

	// ID fields
	if name == "id" {
		return `uuid.New().String()`
	}

	// Handle enums
	if f.IsEnum && len(f.EnumValues) > 0 {
		// Return random valid enum value (skip UNSPECIFIED which is usually 0)
		return fmt.Sprintf("%s(gofakeit.Number(1, %d))", f.EnumName, len(f.EnumValues)-1)
	}

	// String fields - infer from name
	if f.ProtoKind == protoreflect.StringKind {
		return inferStringFaker(name)
	}

	// Numeric fields
	if f.ProtoKind == protoreflect.Int32Kind || f.ProtoKind == protoreflect.Int64Kind {
		return inferIntFaker(name)
	}

	if f.ProtoKind == protoreflect.Uint32Kind || f.ProtoKind == protoreflect.Uint64Kind {
		return inferUintFaker(name)
	}

	if f.ProtoKind == protoreflect.FloatKind || f.ProtoKind == protoreflect.DoubleKind {
		return inferFloatFaker(name)
	}

	// Boolean
	if f.ProtoKind == protoreflect.BoolKind {
		return "gofakeit.Bool()"
	}

	// Bytes
	if f.ProtoKind == protoreflect.BytesKind {
		return "[]byte(gofakeit.LoremIpsumSentence(5))"
	}

	// Default fallback
	return defaultFaker(f.ProtoKind)
}

func inferStringFaker(name string) string {
	// Personal info
	if contains(name, "name", "full_name", "fullname") && !contains(name, "user", "file", "company") {
		return "gofakeit.Name()"
	}
	if contains(name, "first_name", "firstname", "given_name") {
		return "gofakeit.FirstName()"
	}
	if contains(name, "last_name", "lastname", "surname", "family_name") {
		return "gofakeit.LastName()"
	}
	if contains(name, "username", "user_name", "login") {
		return "gofakeit.Username()"
	}
	if contains(name, "email", "mail") {
		return "gofakeit.Email()"
	}
	if contains(name, "phone", "mobile", "cell", "telephone") {
		return "gofakeit.Phone()"
	}
	if contains(name, "password", "passwd", "secret") {
		return "gofakeit.Password(true, true, true, true, false, 12)"
	}

	// Address
	if contains(name, "street", "address_line", "address1", "address2") {
		return "gofakeit.Street()"
	}
	if contains(name, "city") {
		return "gofakeit.City()"
	}
	if contains(name, "state", "province", "region") {
		return "gofakeit.State()"
	}
	if contains(name, "country") && !contains(name, "code") {
		return "gofakeit.Country()"
	}
	if contains(name, "country_code", "countrycode") {
		return "gofakeit.CountryAbr()"
	}
	if contains(name, "zip", "postal", "postcode") {
		return "gofakeit.Zip()"
	}
	if contains(name, "address") {
		return "gofakeit.Address().Address"
	}
	if contains(name, "latitude", "lat") {
		return "fmt.Sprintf(\"%.6f\", gofakeit.Latitude())"
	}
	if contains(name, "longitude", "lng", "lon") {
		return "fmt.Sprintf(\"%.6f\", gofakeit.Longitude())"
	}

	// Company/Business
	if contains(name, "company", "organization", "org_name", "business") {
		return "gofakeit.Company()"
	}
	if contains(name, "job_title", "jobtitle", "title", "position", "role") && !contains(name, "id") {
		return "gofakeit.JobTitle()"
	}
	if contains(name, "industry", "sector") {
		return "gofakeit.RandomString([]string{\"Technology\", \"Healthcare\", \"Finance\", \"Retail\", \"Manufacturing\", \"Education\"})"
	}
	if contains(name, "department", "dept") {
		return "gofakeit.RandomString([]string{\"Engineering\", \"Sales\", \"Marketing\", \"HR\", \"Finance\", \"Operations\"})"
	}

	// Internet/Tech
	if contains(name, "url", "website", "link", "href") {
		return "gofakeit.URL()"
	}
	if contains(name, "domain") {
		return "gofakeit.DomainName()"
	}
	if contains(name, "ip", "ip_address", "ipaddress") {
		return "gofakeit.IPv4Address()"
	}
	if contains(name, "user_agent", "useragent") {
		return "gofakeit.UserAgent()"
	}
	if contains(name, "uuid", "guid") {
		return "uuid.New().String()"
	}
	if contains(name, "slug") {
		return "gofakeit.LoremIpsumWord() + \"-\" + gofakeit.LoremIpsumWord()"
	}
	if contains(name, "token", "api_key", "apikey") {
		return "gofakeit.LetterN(32)"
	}

	// Content
	if contains(name, "description", "desc", "summary", "overview") {
		return "gofakeit.Sentence(15)"
	}
	if contains(name, "bio", "about", "profile") {
		return "gofakeit.Sentence(20)"
	}
	if contains(name, "content", "body", "text", "message") {
		return "gofakeit.Paragraph(2, 3, 10, \" \")"
	}
	if contains(name, "comment", "note", "remark") {
		return "gofakeit.Sentence(10)"
	}
	if contains(name, "title", "headline", "subject") {
		return "gofakeit.Sentence(5)"
	}
	if contains(name, "tag", "label") {
		return "gofakeit.LoremIpsumWord()"
	}

	// Product/Commerce
	if contains(name, "product_name", "productname", "item_name") {
		return "gofakeit.ProductName()"
	}
	if contains(name, "category") {
		return "gofakeit.ProductCategory()"
	}
	if contains(name, "sku", "product_code", "item_code") {
		return "gofakeit.LetterN(3) + \"-\" + gofakeit.DigitN(5)"
	}
	if contains(name, "currency") {
		return "gofakeit.CurrencyShort()"
	}
	if contains(name, "brand") {
		return "gofakeit.Company()"
	}

	// Files/Media
	if contains(name, "filename", "file_name") {
		return "gofakeit.LoremIpsumWord() + \".\" + gofakeit.FileExtension()"
	}
	if contains(name, "mime", "content_type", "contenttype") {
		return "gofakeit.FileMimeType()"
	}
	if contains(name, "image", "photo", "picture", "avatar") && contains(name, "url") {
		return "\"https://picsum.photos/200/200?random=\" + gofakeit.DigitN(5)"
	}
	if contains(name, "image", "photo", "picture", "avatar") {
		return "\"https://picsum.photos/200/200?random=\" + gofakeit.DigitN(5)"
	}
	if contains(name, "color", "colour") {
		return "gofakeit.HexColor()"
	}

	// Dates as strings
	if contains(name, "date") && !contains(name, "created", "updated", "deleted") {
		return "gofakeit.Date().Format(\"2006-01-02\")"
	}
	if contains(name, "time") && !contains(name, "created", "updated", "deleted", "stamp") {
		return "gofakeit.Date().Format(\"15:04:05\")"
	}

	// Code/Reference
	if contains(name, "code") && !contains(name, "country", "postal", "zip") {
		return "gofakeit.LetterN(2) + gofakeit.DigitN(4)"
	}
	if contains(name, "ref", "reference", "number", "num") && !contains(name, "phone") {
		return "gofakeit.DigitN(8)"
	}

	// Locale
	if contains(name, "language", "lang", "locale") {
		return "gofakeit.Language()"
	}
	if contains(name, "timezone", "time_zone", "tz") {
		return "gofakeit.TimeZone()"
	}

	// Default for string
	return "gofakeit.LoremIpsumWord()"
}

func inferIntFaker(name string) string {
	if contains(name, "age") {
		return "int32(gofakeit.Number(18, 80))"
	}
	if contains(name, "year") {
		return "int32(gofakeit.Year())"
	}
	if contains(name, "month") {
		return "int32(gofakeit.Month())"
	}
	if contains(name, "day") {
		return "int32(gofakeit.Day())"
	}
	if contains(name, "hour") {
		return "int32(gofakeit.Hour())"
	}
	if contains(name, "minute", "min") {
		return "int32(gofakeit.Minute())"
	}
	if contains(name, "count", "quantity", "qty", "num", "number") {
		return "int32(gofakeit.Number(1, 100))"
	}
	if contains(name, "price", "cost", "amount", "total") && contains(name, "cent") {
		return "int64(gofakeit.Number(100, 100000))"
	}
	if contains(name, "price", "cost", "amount", "total") {
		return "int32(gofakeit.Number(1, 1000))"
	}
	if contains(name, "rating", "score", "stars") {
		return "int32(gofakeit.Number(1, 5))"
	}
	if contains(name, "percent", "percentage", "pct") {
		return "int32(gofakeit.Number(0, 100))"
	}
	if contains(name, "priority", "level", "rank") {
		return "int32(gofakeit.Number(1, 10))"
	}
	if contains(name, "order", "sequence", "seq", "position", "index") {
		return "int32(gofakeit.Number(1, 1000))"
	}
	if contains(name, "size", "length", "width", "height") {
		return "int32(gofakeit.Number(1, 500))"
	}
	if contains(name, "duration", "seconds", "minutes") {
		return "int32(gofakeit.Number(1, 3600))"
	}
	if contains(name, "port") {
		return "int32(gofakeit.Number(1024, 65535))"
	}
	if contains(name, "version", "major", "minor", "patch") {
		return "int32(gofakeit.Number(0, 20))"
	}

	// Default
	return "int32(gofakeit.Number(1, 1000))"
}

func inferUintFaker(name string) string {
	if contains(name, "count", "quantity", "qty") {
		return "uint32(gofakeit.Number(0, 100))"
	}
	return "uint32(gofakeit.Number(0, 1000))"
}

func inferFloatFaker(name string) string {
	if contains(name, "price", "cost", "amount", "total", "fee") {
		return "gofakeit.Price(1, 1000)"
	}
	if contains(name, "latitude", "lat") {
		return "gofakeit.Latitude()"
	}
	if contains(name, "longitude", "lng", "lon") {
		return "gofakeit.Longitude()"
	}
	if contains(name, "percent", "percentage", "rate", "ratio") {
		return "gofakeit.Float64Range(0, 100)"
	}
	if contains(name, "rating", "score") {
		return "gofakeit.Float64Range(1, 5)"
	}
	if contains(name, "weight", "mass") {
		return "gofakeit.Float64Range(0.1, 100)"
	}
	if contains(name, "temperature", "temp") {
		return "gofakeit.Float64Range(-20, 40)"
	}
	return "gofakeit.Float64Range(0, 1000)"
}

func defaultFaker(kind protoreflect.Kind) string {
	switch kind {
	case protoreflect.StringKind:
		return "gofakeit.LoremIpsumWord()"
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return "int32(gofakeit.Number(1, 1000))"
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return "int64(gofakeit.Number(1, 10000))"
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return "uint32(gofakeit.Number(0, 1000))"
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return "uint64(gofakeit.Number(0, 10000))"
	case protoreflect.FloatKind:
		return "float32(gofakeit.Float64Range(0, 1000))"
	case protoreflect.DoubleKind:
		return "gofakeit.Float64Range(0, 1000)"
	case protoreflect.BoolKind:
		return "gofakeit.Bool()"
	default:
		return "nil"
	}
}

func contains(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// =============================================================================
// CODE GENERATORS
// =============================================================================

func GenerateMockFile(messages []MessageInfo, pkgName string) Code {
	return Concat(CodeMonoid, []Code{
		Line("// Code generated by protoc-gen-mock. DO NOT EDIT."),
		Line("// Fake data generators and seeders for testing"),
		Blank(),
		Linef("package %s", pkgName),
		Blank(),
		Line("import ("),
		Line(`	"context"`),
		Line(`	"fmt"`),
		Blank(),
		Line(`	"github.com/brianvoe/gofakeit/v6"`),
		Line(`	"github.com/google/uuid"`),
		Line(`	"google.golang.org/protobuf/types/known/timestamppb"`),
		Line(")"),
		Blank(),
		Line("func init() {"),
		Line("	gofakeit.Seed(0) // Random seed; use gofakeit.Seed(12345) for reproducible data"),
		Line("}"),
		Blank(),
		FoldMap(messages, CodeMonoid, GenerateFaker),
		FoldMap(messages, CodeMonoid, GenerateFakerN),
		FoldMap(messages, CodeMonoid, GenerateSeeder),
		GenerateSeedAll(messages),
	})
}

func GenerateFaker(m MessageInfo) Code {
	return Concat(CodeMonoid, []Code{
		Linef("// Fake%s generates a fake %s with realistic data", m.Name, m.Name),
		Linef("func Fake%s() *%s {", m.Name, m.Name),
		Linef("	return &%s{", m.Name),
		FoldMap(m.Fields, CodeMonoid, func(f FieldInfo) Code {
			// Skip certain fields
			if isTimestampField(f.Name) && f.MessageRef == "Timestamp" {
				return Linef("		%s: timestamppb.Now(),", f.GoName)
			}
			if f.IsRepeated || f.IsMap {
				return CodeMonoid.Empty() // Skip repeated/map for now
			}
			if f.MessageRef != "" && f.MessageRef != "Timestamp" {
				return CodeMonoid.Empty() // Skip nested messages for now
			}
			faker := InferFaker(f)
			if faker == "nil" {
				return CodeMonoid.Empty()
			}
			return Linef("		%s: %s,", f.GoName, faker)
		}),
		Line("	}"),
		Line("}"),
		Blank(),
	})
}

func GenerateFakerN(m MessageInfo) Code {
	return Concat(CodeMonoid, []Code{
		Linef("// Fake%ss generates n fake %s instances", m.Name, m.Name),
		Linef("func Fake%ss(n int) []*%s {", m.Name, m.Name),
		Linef("	result := make([]*%s, n)", m.Name),
		Line("	for i := 0; i < n; i++ {"),
		Linef("		result[i] = Fake%s()", m.Name),
		Line("	}"),
		Line("	return result"),
		Line("}"),
		Blank(),
	})
}

func GenerateSeeder(m MessageInfo) Code {
	lower := lowerFirst(m.Name)
	return Concat(CodeMonoid, []Code{
		Linef("// Seed%ss seeds the repository with n fake %s instances", m.Name, m.Name),
		Linef("func Seed%ss(ctx context.Context, repo %sRepository, n int) ([]*%s, error) {", m.Name, m.Name, m.Name),
		Linef("	var created []*%s", m.Name),
		Line("	for i := 0; i < n; i++ {"),
		Linef("		%s := Fake%s()", lower, m.Name),
		Linef("		id, err := repo.Create(ctx, %s)", lower),
		Line("		if err != nil {"),
		Linef("			return created, fmt.Errorf(\"failed to seed %s %%d: %%w\", i, err)", m.Name),
		Line("		}"),
		Linef("		%s.Id = id", lower),
		Linef("		created = append(created, %s)", lower),
		Line("	}"),
		Line("	return created, nil"),
		Line("}"),
		Blank(),
	})
}

func GenerateSeedAll(messages []MessageInfo) Code {
	return Concat(CodeMonoid, []Code{
		Line("// SeedAll seeds all repositories with fake data"),
		Line("type Repositories struct {"),
		FoldMap(messages, CodeMonoid, func(m MessageInfo) Code {
			return Linef("	%s %sRepository", m.Name, m.Name)
		}),
		Line("}"),
		Blank(),
		Line("// SeedCounts specifies how many of each entity to create"),
		Line("type SeedCounts struct {"),
		FoldMap(messages, CodeMonoid, func(m MessageInfo) Code {
			return Linef("	%s int", m.Name)
		}),
		Line("}"),
		Blank(),
		Line("// DefaultSeedCounts returns sensible defaults"),
		Line("func DefaultSeedCounts() SeedCounts {"),
		Line("	return SeedCounts{"),
		FoldMap(messages, CodeMonoid, func(m MessageInfo) Code {
			return Linef("		%s: 10,", m.Name)
		}),
		Line("	}"),
		Line("}"),
		Blank(),
		Line("// SeedAllRepositories seeds all repositories with fake data"),
		Line("func SeedAllRepositories(ctx context.Context, repos Repositories, counts SeedCounts) error {"),
		FoldMap(messages, CodeMonoid, func(m MessageInfo) Code {
			return Concat(CodeMonoid, []Code{
				Linef("	if repos.%s != nil && counts.%s > 0 {", m.Name, m.Name),
				Linef("		if _, err := Seed%ss(ctx, repos.%s, counts.%s); err != nil {", m.Name, m.Name, m.Name),
				Line("			return err"),
				Line("		}"),
				Line("	}"),
			})
		}),
		Line("	return nil"),
		Line("}"),
		Blank(),
	})
}

func isTimestampField(name string) bool {
	lower := strings.ToLower(name)
	return contains(lower, "created_at", "updated_at", "deleted_at", "timestamp", "_at")
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

			// Extract messages with ID field (entities)
			messages := Filter(
				Map(f.Messages, ExtractMessageInfo),
				func(m MessageInfo) bool {
					for _, field := range m.Fields {
						if strings.EqualFold(field.Name, "id") {
							return true
						}
					}
					return false
				},
			)

			if len(messages) == 0 {
				continue
			}

			pkgName := string(f.GoPackageName)

			// Generate mock file
			mockFile := gen.NewGeneratedFile(f.GeneratedFilenamePrefix+"_mock.pb.go", f.GoImportPath)
			mockFile.P(GenerateMockFile(messages, pkgName).Run())
		}
		return nil
	})
}
