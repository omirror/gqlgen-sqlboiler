package gbgen

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"

	"github.com/web-ridge/gqlgen-sqlboiler/v3/structs"

	"github.com/web-ridge/gqlgen-sqlboiler/v3/cache"

	"github.com/web-ridge/gqlgen-sqlboiler/v3/customization"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/web-ridge/gqlgen-sqlboiler/v3/templates"
)

var pathRegex *regexp.Regexp //nolint:gochecknoglobals

func init() { //nolint:gochecknoinits
	pathRegex = regexp.MustCompile(`src/(.*)`)

	// Default level for this example is info, unless debug flag is present
	zerolog.SetGlobalLevel(zerolog.DebugLevel)

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
}

type Import struct {
	Alias      string
	ImportPath string
}

type ConvertTemplateData struct {
	Backend      structs.Config
	Frontend     structs.Config
	PluginConfig ConvertPluginConfig
	PackageName  string
	Interfaces   []*structs.Interface
	Models       []*structs.Model
	Enums        []*structs.Enum
	Scalars      []string
}

func (t ConvertTemplateData) Imports() []Import {
	return []Import{
		{
			Alias:      t.Frontend.PackageName,
			ImportPath: t.Frontend.Directory,
		},
		{
			Alias:      t.Backend.PackageName,
			ImportPath: t.Backend.Directory,
		},
	}
}

func NewConvertPlugin(modelCache *cache.ModelCache, pluginConfig ConvertPluginConfig) *ConvertPlugin {
	return &ConvertPlugin{
		ModelCache:     modelCache,
		PluginConfig:   pluginConfig,
		rootImportPath: getRootImportPath(),
	}
}

type ConvertPlugin struct {
	BoilerCache    *cache.BoilerCache
	ModelCache     *cache.ModelCache
	PluginConfig   ConvertPluginConfig
	rootImportPath string
}

// DatabaseDriver defines which data syntax to use for some of the converts
type DatabaseDriver string

const (
	// MySQL is the default
	MySQL DatabaseDriver = "mysql"
	// PostgreSQL is the default
	PostgreSQL DatabaseDriver = "postgres"
)

type ConvertPluginConfig struct {
	DatabaseDriver DatabaseDriver
}

func (m *ConvertPlugin) GenerateCode() error {
	data := &ConvertTemplateData{
		PackageName: m.ModelCache.Output.PackageName,
		Backend: structs.Config{
			Directory:   path.Join(m.rootImportPath, m.ModelCache.Backend.Directory),
			PackageName: m.ModelCache.Backend.PackageName,
		},
		Frontend: structs.Config{
			Directory:   path.Join(m.rootImportPath, m.ModelCache.Frontend.Directory),
			PackageName: m.ModelCache.Frontend.PackageName,
		},
		PluginConfig: m.PluginConfig,
		Interfaces:   m.ModelCache.Interfaces,
		Models:       m.ModelCache.Models,
		Enums:        m.ModelCache.Enums,
		Scalars:      m.ModelCache.Scalars,
	}

	if err := os.MkdirAll(m.ModelCache.Output.Directory, os.ModePerm); err != nil {
		log.Error().Err(err).Str("directory", m.ModelCache.Output.Directory).Msg("could not create directories")
	}

	if m.PluginConfig.DatabaseDriver == "" {
		fmt.Println("Please specify database driver, see README on github")
	}

	if len(m.ModelCache.Models) == 0 {
		log.Warn().Msg("no structs found in graphql so skipping generation")
		return nil
	}

	filesToGenerate := []string{
		"generated_convert.go",
		"generated_convert_batch.go",
		"generated_convert_input.go",
		"generated_filter.go",
		"generated_preload.go",
		"generated_filter_parser.go",
		"generated_sort.go",
	}

	// We get all function names from helper repository to check if any customizations are available
	// we ignore the files we generated by this plugin
	userDefinedFunctions, err := customization.GetFunctionNamesFromDir(m.ModelCache.Output.PackageName, filesToGenerate)
	if err != nil {
		log.Err(err).Msg("could not parse user defined functions")
	}

	for _, fn := range filesToGenerate {
		m.generateFile(data, fn, userDefinedFunctions)
	}
	return nil
}

func (m *ConvertPlugin) generateFile(data *ConvertTemplateData, fileName string, userDefinedFunctions []string) {
	templateName := fileName + "tpl"
	// log.Debug().Msg("[convert] render " + templateName)

	templateContent, err := getTemplateContent(templateName)
	if err != nil {
		log.Err(err).Msg("error when reading " + templateName)
	}

	if renderError := templates.WriteTemplateFile(
		m.ModelCache.Output.Directory+"/"+fileName,
		templates.Options{
			Template:             templateContent,
			PackageName:          m.ModelCache.Output.PackageName,
			Data:                 data,
			UserDefinedFunctions: userDefinedFunctions,
		}); renderError != nil {
		log.Err(renderError).Msg("error while rendering " + templateName)
	}
	log.Debug().Msg("[convert] generated " + templateName)
}

func getTemplateContent(filename string) (string, error) {
	// load path relative to calling source file
	_, callerFile, _, _ := runtime.Caller(1) //nolint:dogsled
	rootDir := filepath.Dir(callerFile)
	content, err := ioutil.ReadFile(path.Join(rootDir, "template_files", filename))
	if err != nil {
		return "", fmt.Errorf("could not read template file: %v", err)
	}
	return string(content), nil
}
