package main

import (
	"github.com/vinodhalaharvi/buf-go-plugins/cmd/protoc-gen-category/internal/generator"
	"google.golang.org/protobuf/compiler/protogen"
)

func main() {
	protogen.Options{}.Run(func(gen *protogen.Plugin) error {
		g := generator.New(gen)
		for _, f := range gen.Files {
			if !f.Generate {
				continue
			}
			if err := g.GenerateFile(f); err != nil {
				return err
			}
		}
		return nil
	})
}
