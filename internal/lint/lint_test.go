package lint

import (
	"testing"
)

const validPipeline = `
apiVersion: core.rageta.io/v1beta1
kind: Pipeline
metadata:
  name: example
entrypoint: build
tasks:
- name: build
  timeout: 30s
  steps:
  - script: echo hello
`

func TestValidatePipelineValid(t *testing.T) {
	if err := Validate([]byte(validPipeline)); err != nil {
		t.Fatalf("expected valid pipeline, got error: %v", err)
	}
}

func TestValidatePipelineWrongTypes(t *testing.T) {
	manifest := `
apiVersion: core.rageta.io/v1beta1
kind: Pipeline
metadata:
  name: example
entrypoint: 123
tasks: "not-a-list"
`
	err := Validate([]byte(manifest))
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

func TestValidatePipelineWrongKind(t *testing.T) {
	manifest := `
apiVersion: core.rageta.io/v1beta1
kind: NotAPipeline
metadata:
  name: example
`
	err := Validate([]byte(manifest))
	if err == nil {
		t.Fatal("expected error for wrong kind, got nil")
	}
}

func TestValidatePipelineUnsupportedVersion(t *testing.T) {
	manifest := `
apiVersion: core.rageta.io/v9999
kind: Pipeline
metadata:
  name: example
`
	err := Validate([]byte(manifest))
	if err == nil {
		t.Fatal("expected error for unsupported apiVersion, got nil")
	}
}

func TestValidatePipelineInvalidYAML(t *testing.T) {
	err := Validate([]byte("not: [valid: yaml"))
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}
