package generator

import (
	"bytes"
	"flag"
	"fmt"
	"go/format"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	DefaultPackageName  = "errcode"
	DefaultErrorxImport = "github.com/fino-io/finokit/errorx"
)

var (
	goExportedIdentPattern = regexp.MustCompile(`^[A-Z][A-Za-z0-9_]*$`)
	goPackagePattern       = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

type Config struct {
	Inputs       []string
	OutputDir    string
	PackageName  string
	ErrorxImport string
}

type Definition struct {
	Code        int    `yaml:"code"`
	Name        string `yaml:"name"`
	Message     string `yaml:"message,omitempty"`
	Description string `yaml:"description,omitempty"`
	CountInSLA  *bool  `yaml:"countInSLA,omitempty"`
	HTTPStatus  int    `yaml:"httpStatus,omitempty"`
}

type File struct {
	AppCode   int          `yaml:"appCode"`
	BizCode   int          `yaml:"bizCode"`
	ErrorCode []Definition `yaml:"errorCode"`
}

type inputFile struct {
	Path     string
	BizName  string
	FileName string
	Spec     File
}

func Run(args []string, stdout io.Writer, stderr io.Writer) error {
	cfg, err := ParseArgs(args, stderr)
	if err != nil {
		return err
	}

	outputPaths, err := Generate(cfg)
	if err != nil {
		return err
	}

	for _, outputPath := range outputPaths {
		_, _ = fmt.Fprintln(stdout, outputPath)
	}
	return nil
}

func ParseArgs(args []string, stderr io.Writer) (Config, error) {
	flagSet := flag.NewFlagSet("errorxgen", flag.ContinueOnError)
	flagSet.SetOutput(io.Discard)

	cfg := Config{
		OutputDir:    ".",
		PackageName:  DefaultPackageName,
		ErrorxImport: DefaultErrorxImport,
	}
	flagSet.StringVar(&cfg.OutputDir, "out", cfg.OutputDir, "output directory")
	flagSet.StringVar(&cfg.PackageName, "pkg", cfg.PackageName, "generated package name")
	flagSet.StringVar(&cfg.ErrorxImport, "errorx-import", cfg.ErrorxImport, "errorx import path")

	if err := flagSet.Parse(args); err != nil {
		printUsage(stderr)
		return Config{}, err
	}

	cfg.Inputs = flagSet.Args()
	if err := validateConfig(cfg); err != nil {
		printUsage(stderr)
		return Config{}, err
	}
	return cfg, nil
}

func Generate(cfg Config) ([]string, error) {
	if strings.TrimSpace(cfg.OutputDir) == "" {
		cfg.OutputDir = "."
	}
	if strings.TrimSpace(cfg.PackageName) == "" {
		cfg.PackageName = DefaultPackageName
	}
	if strings.TrimSpace(cfg.ErrorxImport) == "" {
		cfg.ErrorxImport = DefaultErrorxImport
	}
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	files, err := collectYAMLFiles(cfg.Inputs)
	if err != nil {
		return nil, err
	}

	inputFiles := make([]inputFile, 0, len(files))
	for _, path := range files {
		spec, err := loadSpec(path)
		if err != nil {
			return nil, err
		}
		if err := validateSpec(path, spec); err != nil {
			return nil, err
		}
		bizName := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		inputFiles = append(inputFiles, inputFile{
			Path:     path,
			BizName:  bizName,
			FileName: sanitizeFileName(bizName),
			Spec:     spec,
		})
	}
	if err := validateInputFiles(inputFiles); err != nil {
		return nil, err
	}

	outputPaths := make([]string, 0, len(inputFiles))
	for _, inputFile := range inputFiles {
		outputPath, err := generateGoCode(inputFile, cfg)
		if err != nil {
			return nil, err
		}
		outputPaths = append(outputPaths, outputPath)
	}
	return outputPaths, nil
}

func validateConfig(cfg Config) error {
	if len(cfg.Inputs) == 0 {
		return fmt.Errorf("at least one yaml file or directory is required")
	}
	if !goPackagePattern.MatchString(cfg.PackageName) {
		return fmt.Errorf("invalid package name %q", cfg.PackageName)
	}
	if strings.TrimSpace(cfg.ErrorxImport) == "" {
		return fmt.Errorf("errorx import path is required")
	}
	return nil
}

func printUsage(w io.Writer) {
	if w == nil {
		return
	}
	_, _ = fmt.Fprintln(w, "usage: errorxgen [-out output-dir] [-pkg package] [-errorx-import import-path] <yaml-file-or-dir> [more-yaml-files-or-dirs...]")
	_, _ = fmt.Fprintln(w, "yaml format: appCode, bizCode, errorCode")
	_, _ = fmt.Fprintln(w, "example: go run github.com/fino-io/fino/errorx/cmd/errorxgen -out ./internal/errcode -pkg errcode ./configs/error_code")
}

func collectYAMLFiles(inputs []string) ([]string, error) {
	var files []string
	for _, input := range inputs {
		info, err := os.Stat(input)
		if err != nil {
			return nil, err
		}

		if !info.IsDir() {
			if !isYAMLFile(input) {
				return nil, fmt.Errorf("unsupported file type: %s", input)
			}
			files = append(files, input)
			continue
		}

		count := 0
		err = filepath.WalkDir(input, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() || !isYAMLFile(path) {
				return nil
			}
			files = append(files, path)
			count++
			return nil
		})
		if err != nil {
			return nil, err
		}
		if count == 0 {
			return nil, fmt.Errorf("no yaml files found in %s", input)
		}
	}

	sort.Strings(files)
	return files, nil
}

func loadSpec(path string) (File, error) {
	var spec File

	data, err := os.ReadFile(path)
	if err != nil {
		return spec, fmt.Errorf("read %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return spec, fmt.Errorf("unmarshal %s: %w", path, err)
	}
	return spec, nil
}

func validateSpec(path string, spec File) error {
	if spec.AppCode < 1 || spec.AppCode > 9 {
		return fmt.Errorf("%s: appCode must be in [1,9]", path)
	}
	if spec.BizCode < 1 || spec.BizCode > 9999 {
		return fmt.Errorf("%s: bizCode must be in [1,9999]", path)
	}
	if len(spec.ErrorCode) == 0 {
		return fmt.Errorf("%s: errorCode cannot be empty", path)
	}

	seenNames := make(map[string]struct{}, len(spec.ErrorCode))
	seenCodes := make(map[int]struct{}, len(spec.ErrorCode))
	for _, errDef := range spec.ErrorCode {
		if !goExportedIdentPattern.MatchString(errDef.Name) {
			return fmt.Errorf("%s: invalid error name %q: must be an exported Go identifier", path, errDef.Name)
		}
		if errDef.Code < 1 || errDef.Code > 9999 {
			return fmt.Errorf("%s: invalid error code for %s: must be in [1,9999]", path, errDef.Name)
		}
		if errDef.HTTPStatus != 0 && (errDef.HTTPStatus < 100 || errDef.HTTPStatus > 599) {
			return fmt.Errorf("%s: invalid httpStatus for %s: must be in [100,599]", path, errDef.Name)
		}
		if _, ok := seenNames[errDef.Name]; ok {
			return fmt.Errorf("%s: duplicate error name %q", path, errDef.Name)
		}
		if _, ok := seenCodes[errDef.Code]; ok {
			return fmt.Errorf("%s: duplicate error sub-code %d", path, errDef.Code)
		}
		seenNames[errDef.Name] = struct{}{}
		seenCodes[errDef.Code] = struct{}{}
	}

	return nil
}

func validateInputFiles(inputFiles []inputFile) error {
	seenFiles := make(map[string]string, len(inputFiles))
	seenNames := make(map[string]string)
	seenCodes := make(map[int]string)

	for _, inputFile := range inputFiles {
		outputName := inputFile.FileName + ".go"
		if previousPath, ok := seenFiles[outputName]; ok {
			return fmt.Errorf("output file conflict: %s and %s both map to %s", previousPath, inputFile.Path, outputName)
		}
		seenFiles[outputName] = inputFile.Path

		for _, errDef := range inputFile.Spec.ErrorCode {
			if previousPath, ok := seenNames[errDef.Name]; ok {
				return fmt.Errorf("duplicate error name %q across files: %s and %s", errDef.Name, previousPath, inputFile.Path)
			}
			seenNames[errDef.Name] = inputFile.Path

			fullCode := composeErrorCode(inputFile.Spec.AppCode, inputFile.Spec.BizCode, errDef.Code)
			location := fmt.Sprintf("%s:%s", inputFile.Path, errDef.Name)
			if previousLocation, ok := seenCodes[fullCode]; ok {
				return fmt.Errorf("duplicate full error code %d: %s and %s", fullCode, previousLocation, location)
			}
			seenCodes[fullCode] = location
		}
	}

	return nil
}

func generateGoCode(inputFile inputFile, cfg Config) (string, error) {
	source, err := buildGoCode(inputFile.BizName, inputFile.Spec, cfg)
	if err != nil {
		return "", err
	}

	outputPath := filepath.Join(cfg.OutputDir, inputFile.FileName+".go")
	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(outputPath, source, 0o644); err != nil {
		return "", err
	}

	return outputPath, nil
}

func buildGoCode(bizName string, spec File, cfg Config) ([]byte, error) {
	var buf bytes.Buffer

	_, _ = fmt.Fprintf(&buf, `// Code generated by errorxgen. DO NOT EDIT.
// biz: %s

package %s

import (
	"%s"
)

`, bizName, cfg.PackageName, cfg.ErrorxImport)

	for _, errDef := range spec.ErrorCode {
		writeDefinition(&buf, spec.AppCode, spec.BizCode, errDef)
		writeFunctions(&buf, errDef)
	}

	source, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("format generated code: %w", err)
	}
	return source, nil
}

func writeDefinition(buf *bytes.Buffer, appCode int, bizCode int, errDef Definition) {
	comment := ""
	if errDef.Description != "" {
		comment = "// " + errDef.Description + "\n"
	}
	_, _ = fmt.Fprintf(buf, "%svar %s = errorx.Define(\n\t%d,\n\t%q,\n\terrorx.CountInSLA(%t),\n", comment, errDef.Name, composeErrorCode(appCode, bizCode, errDef.Code), errDef.Message, errDef.countInSLA())
	if errDef.HTTPStatus != 0 {
		_, _ = fmt.Fprintf(buf, "\terrorx.HTTPStatus(%d),\n", errDef.HTTPStatus)
	}
	_, _ = fmt.Fprint(buf, ")\n\n")
}

func writeFunctions(buf *bytes.Buffer, errDef Definition) {
	_, _ = fmt.Fprintf(
		buf,
		"func New%s(opts ...errorx.Option) error {\n\treturn %s.New(opts...)\n}\n\n",
		errDef.Name,
		errDef.Name,
	)
	_, _ = fmt.Fprintf(
		buf,
		"func Wrap%s(err error, opts ...errorx.Option) error {\n\treturn %s.Wrap(err, opts...)\n}\n\n",
		errDef.Name,
		errDef.Name,
	)
	_, _ = fmt.Fprintf(
		buf,
		"func Is%s(err error) bool {\n\treturn %s.Is(err)\n}\n\n",
		errDef.Name,
		errDef.Name,
	)
}

func sanitizeFileName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	name = strings.NewReplacer("-", "_", ".", "_", "/", "_", " ", "_").Replace(name)
	if name == "" {
		return "error_code"
	}
	if name[0] >= '0' && name[0] <= '9' {
		return "biz_" + name
	}
	return name
}

func (e Definition) countInSLA() bool {
	if e.CountInSLA != nil {
		return *e.CountInSLA
	}
	return true
}

func isYAMLFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yaml" || ext == ".yml"
}

func composeErrorCode(appCode int, bizCode int, subCode int) int {
	return appCode*100000000 + bizCode*10000 + subCode
}
