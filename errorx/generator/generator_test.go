package generator

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateWritesObjectDefinitions(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()
	writeFile(t, filepath.Join(inputDir, "loop_task.yaml"), `
appCode: 6
bizCode: 12
errorCode:
  - name: TaskNotFound
    code: 1001
    message: task not found
    description: task does not exist
    countInSLA: false
    httpStatus: 404
`)

	outputs, err := Generate(Config{
		Inputs:       []string{inputDir},
		OutputDir:    outputDir,
		PackageName:  "bizerr",
		ErrorxImport: "example.com/platform/errorx",
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if len(outputs) != 1 {
		t.Fatalf("unexpected outputs: %v", outputs)
	}

	content, err := os.ReadFile(filepath.Join(outputDir, "loop_task.go"))
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	got := string(content)
	for _, want := range []string{
		"package bizerr",
		`"example.com/platform/errorx"`,
		"var TaskNotFound = errorx.Define(",
		"600121001,",
		`"task not found",`,
		"errorx.CountInSLA(false),",
		"errorx.HTTPStatus(404),",
		"func NewTaskNotFound(opts ...errorx.Option) error",
		"return TaskNotFound.New(opts...)",
		"func WrapTaskNotFound(err error, opts ...errorx.Option) error",
		"return TaskNotFound.Wrap(err, opts...)",
		"func IsTaskNotFound(err error) bool",
		"return TaskNotFound.Is(err)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("generated file missing %q:\n%s", want, got)
		}
	}

	if _, err := os.Stat(filepath.Join(outputDir, "register_all.go")); !os.IsNotExist(err) {
		t.Fatalf("register_all.go should not be generated: %v", err)
	}
}

func TestRunPrintsGeneratedFiles(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()
	writeFile(t, filepath.Join(inputDir, "user.yaml"), `
appCode: 6
bizCode: 13
errorCode:
  - name: UserNotFound
    code: 1001
    message: user not found
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := Run([]string{"-out", outputDir, "-pkg", "bizerr", inputDir}, &stdout, &stderr); err != nil {
		t.Fatalf("Run returned error: %v; stderr=%s", err, stderr.String())
	}
	lines := strings.Fields(strings.TrimSpace(stdout.String()))
	if len(lines) != 1 {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
}

func TestGenerateWritesOneMarkdownDocumentWithPlatformFirst(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()
	docPath := filepath.Join(t.TempDir(), "docs", "error-codes.md")
	writeFile(t, filepath.Join(inputDir, "z_task.yaml"), `
appCode: 6
bizCode: 12
errorCode:
  - name: TaskNotFound
    code: 1001
    message: "task | not found"
    description: task does not exist
`)
	writeFile(t, filepath.Join(inputDir, "platform.yaml"), `
appCode: 6
bizCode: 1
errorCode:
  - name: InvalidArgument
    code: 1001
`)
	writeFile(t, filepath.Join(inputDir, "account.yaml"), `
appCode: 6
bizCode: 2
errorCode:
  - name: AccountDisabled
    code: 1001
    countInSLA: false
    httpStatus: 403
`)

	outputs, err := Generate(Config{
		Inputs:      []string{inputDir},
		OutputDir:   outputDir,
		DocOutput:   docPath,
		PackageName: "errcode",
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if len(outputs) != 4 || outputs[len(outputs)-1] != docPath {
		t.Fatalf("unexpected outputs: %v", outputs)
	}

	content, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	got := string(content)
	platformIndex := strings.Index(got, "## platform")
	accountIndex := strings.Index(got, "## account")
	taskIndex := strings.Index(got, "## z_task")
	if platformIndex == -1 || accountIndex == -1 || taskIndex == -1 || platformIndex > accountIndex || accountIndex > taskIndex {
		t.Fatalf("unexpected document order:\n%s", got)
	}
	for _, want := range []string{
		"| platform | platform.yaml | 6 | 1 | 1 | 600011001 - 600011001 |",
		"| 600121001 | TaskNotFound | task \\| not found | task does not exist | 500（默认） | 是（默认） |",
		"| 600021001 | AccountDisabled | Service Internal Error（默认） | — | 403 | 否 |",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("document missing %q:\n%s", want, got)
		}
	}
}

func TestValidateRejectsDuplicatesAndInvalidNames(t *testing.T) {
	err := validateInputFiles([]inputFile{
		{
			Path:     "order.yaml",
			BizName:  "order",
			FileName: "order",
			Spec: File{
				AppCode: 6,
				BizCode: 12,
				ErrorCode: []Definition{
					{Name: "NotFound", Code: 1001},
				},
			},
		},
		{
			Path:     "payment.yaml",
			BizName:  "payment",
			FileName: "payment",
			Spec: File{
				AppCode: 6,
				BizCode: 12,
				ErrorCode: []Definition{
					{Name: "PaymentNotFound", Code: 1001},
				},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate full error code 600121001") {
		t.Fatalf("unexpected duplicate code error: %v", err)
	}

	err = validateSpec("bad.yaml", File{
		AppCode: 6,
		BizCode: 12,
		ErrorCode: []Definition{
			{Name: "taskNotFound", Code: 1001},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "exported Go identifier") {
		t.Fatalf("unexpected invalid name error: %v", err)
	}

	err = validateSpec("bad.yaml", File{
		AppCode:   6,
		BizCode:   12,
		ErrorCode: []Definition{{Name: "TaskNotFound", Code: 1001, HTTPStatus: 99}},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid httpStatus") {
		t.Fatalf("unexpected invalid http status error: %v", err)
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
}
