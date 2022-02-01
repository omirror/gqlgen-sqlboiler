package templates

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"strings"
	"text/template"

	"github.com/rs/zerolog/log"

	"github.com/iancoleman/strcase"

	"golang.org/x/tools/imports"

	gqlgenTemplates "github.com/99designs/gqlgen/codegen/templates"
)

type Options struct {
	// PackageName is a helper that specifies the package header declaration.
	// In other words, when you write the template you don't need to specify `package X`
	// at the top of the file. By providing PackageName in the Options, the Render
	// function will do that for you.
	PackageName string
	// Template is a string of the entire template that
	// will be parsed and rendered. If it's empty,
	// the plugin processor will look for .gotpl files
	// in the same directory of where you wrote the plugin.
	Template string
	// UserDefinedFunctions is used to rewrite in the the file so we can use custom functions
	// The struct is still available for use in private but will be rewritten to
	// a private function with original in front of it
	UserDefinedFunctions []string
	// Data will be passed to the template execution.
	Data interface{}
}

func init() { // nolint:gochecknoinits
	strcase.ConfigureAcronym("QR", "qr")
	strcase.ConfigureAcronym("KVK", "kvk")
	strcase.ConfigureAcronym("URL", "url")
}

func WriteTemplateFile(fileName string, cfg Options) error {
	content, contentError := GetTemplateContent(cfg)
	importFixedContent, importsError := imports.Process(fileName, []byte(content), nil)

	fSet := token.NewFileSet()
	node, err := parser.ParseFile(fSet, "src.go", string(importFixedContent), 0)
	if err != nil {
		log.Error().Err(err).Msg("could not parse golang file")
	}

	ast.Inspect(node, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if ok && isFunctionOverriddenByUser(fn.Name.Name, cfg.UserDefinedFunctions) {
			fn.Name.Name = "original" + fn.Name.Name
		}
		return true
	})

	// write new ast to file
	f, writeError := os.Create(fileName)
	defer func() {
		if err := f.Close(); err != nil {
			log.Error().Err(err).Str("fileName", fileName).Msg("could not close file")
		}
	}()

	if err := printer.Fprint(f, fSet, node); err != nil {
		return fmt.Errorf("errors while printing template to %v  %v", fileName, err)
	}

	if contentError != nil || writeError != nil || importsError != nil {
		return fmt.Errorf(
			"errors while writing template to %v writeError: %v, contentError: %v, importError: %v",
			fileName, writeError, contentError, importsError)
	}

	return nil
}

func GetTemplateContent(cfg Options) (string, error) {
	tpl, err := template.New("").Funcs(template.FuncMap{
		"go":      gqlgenTemplates.ToGo,
		"id":      ToGoId,
		"lcFirst": gqlgenTemplates.LcFirst,
		"ucFirst": gqlgenTemplates.UcFirst,
	}).Parse(cfg.Template)
	if err != nil {
		return "", fmt.Errorf("parse: %v", err)
	}

	var content bytes.Buffer
	err = tpl.Execute(&content, cfg.Data)
	if err != nil {
		return "", fmt.Errorf("execute: %v", err)
	}

	contentBytes := content.Bytes()
	formattedContent, err := format.Source(contentBytes)
	if err != nil {
		return string(contentBytes), fmt.Errorf("formatting: %v", err)
	}

	return string(formattedContent), nil
}

func isFunctionOverriddenByUser(functionName string, userDefinedFunctions []string) bool {
	for _, userDefinedFunction := range userDefinedFunctions {
		if userDefinedFunction == functionName {
			return true
		}
	}
	return false
}

func ToGo(name string) string {
	return strcase.ToCamel(name)
}

func ToGoId(str string) string {
	name := gqlgenTemplates.ToGo(str)

	if strings.HasSuffix(strings.ToLower(name), "id") {
		return strings.Replace(name, "ID", "Id", -1)
	}

	if strings.HasSuffix(strings.ToLower(name), "ids") {
		return strings.Replace(name, "IDS", "Ids", -1)
	}

	return name
}

func ToLowerAndGo(name string) string {
	return ToGo(strings.ToLower(name))
}
