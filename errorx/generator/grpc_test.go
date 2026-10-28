package generator

import (
	"bytes"
	"strings"
	"testing"
)

func TestGRPCCode(t *testing.T) {
	spec := File{AppCode: 1, BizCode: 1, ErrorCode: []Definition{{Name: "Conflict", Code: 1, GRPCCode: "Aborted"}}}
	if err := validateSpec("test", spec); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	writeDefinition(&out, 1, 1, spec.ErrorCode[0])
	if !strings.Contains(out.String(), `errorx.GRPCCode("Aborted")`) {
		t.Fatal(out.String())
	}
	spec.ErrorCode[0].GRPCCode = "Bogus"
	if validateSpec("test", spec) == nil {
		t.Fatal("invalid grpcCode accepted")
	}
}
