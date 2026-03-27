package pipeline

import (
	"testing"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveExtends_NoExtends(t *testing.T) {
	steps := []v1beta1.Step{
		{Name: "a", Run: &v1beta1.RunStep{Container: v1beta1.Container{Image: "alpine"}}},
		{Name: "b", Run: &v1beta1.RunStep{Container: v1beta1.Container{Image: "ubuntu"}}},
	}

	resolved, err := resolveExtends(steps)
	require.NoError(t, err)
	assert.Equal(t, steps, resolved)
}

func TestResolveExtends_SimpleExtends(t *testing.T) {
	template := v1beta1.Step{
		Name: "base",
		Run: &v1beta1.RunStep{
			Container: v1beta1.Container{
				Image:  "alpine",
				Script: "echo base",
			},
		},
		StepOptions: v1beta1.StepOptions{
			AllowFailure: true,
			Env: []v1beta1.EnvVar{
				{Name: "FOO", Value: strPtr("bar")},
			},
		},
	}

	Extendsing := v1beta1.Step{
		Name: "child",
		StepOptions: v1beta1.StepOptions{
			Extends: &v1beta1.StepReference{Name: "base"},
			Env: []v1beta1.EnvVar{
				{Name: "EXTRA", Value: strPtr("val")},
			},
		},
	}

	resolved, err := resolveExtends([]v1beta1.Step{template, Extendsing})
	require.NoError(t, err)
	require.Len(t, resolved, 2)

	child := resolved[1]
	assert.Equal(t, "child", child.Name)
	assert.Nil(t, child.Extends, "Extends should be cleared after resolution")
	assert.Equal(t, "alpine", child.Run.Image)
	assert.Equal(t, "echo base", child.Run.Script)
	assert.True(t, child.AllowFailure)
	// Env from the Extendsing step overrides (merge patch replaces arrays)
	assert.Equal(t, []v1beta1.EnvVar{{Name: "EXTRA", Value: strPtr("val")}}, child.Env)
}

func TestResolveExtends_OverrideFields(t *testing.T) {
	template := v1beta1.Step{
		Name: "base",
		Run: &v1beta1.RunStep{
			Container: v1beta1.Container{Image: "alpine", Script: "echo template"},
		},
	}

	Extendsing := v1beta1.Step{
		Name: "child",
		StepOptions: v1beta1.StepOptions{
			Extends: &v1beta1.StepReference{Name: "base"},
		},
		Run: &v1beta1.RunStep{
			Container: v1beta1.Container{Image: "ubuntu"},
		},
	}

	resolved, err := resolveExtends([]v1beta1.Step{template, Extendsing})
	require.NoError(t, err)

	child := resolved[1]
	assert.Equal(t, "ubuntu", child.Run.Image)
	// Script from template is kept since Extendsing step did not set it
	assert.Equal(t, "echo template", child.Run.Script)
}

func TestResolveExtends_ChainedExtends(t *testing.T) {
	grandparent := v1beta1.Step{
		Name: "grandparent",
		Run:  &v1beta1.RunStep{Container: v1beta1.Container{Image: "alpine"}},
		StepOptions: v1beta1.StepOptions{
			AllowFailure: true,
		},
	}

	parent := v1beta1.Step{
		Name: "parent",
		StepOptions: v1beta1.StepOptions{
			Extends: &v1beta1.StepReference{Name: "grandparent"},
			Env:     []v1beta1.EnvVar{{Name: "LEVEL", Value: strPtr("parent")}},
		},
	}

	child := v1beta1.Step{
		Name: "child",
		StepOptions: v1beta1.StepOptions{
			Extends: &v1beta1.StepReference{Name: "parent"},
			Env:     []v1beta1.EnvVar{{Name: "LEVEL", Value: strPtr("child")}},
		},
	}

	resolved, err := resolveExtends([]v1beta1.Step{grandparent, parent, child})
	require.NoError(t, err)

	childStep := resolved[2]
	assert.Equal(t, "child", childStep.Name)
	assert.Nil(t, childStep.Extends)
	assert.Equal(t, "alpine", childStep.Run.Image)
	assert.True(t, childStep.AllowFailure)
	assert.Equal(t, []v1beta1.EnvVar{{Name: "LEVEL", Value: strPtr("child")}}, childStep.Env)
}

func TestResolveExtends_UnknownTemplate(t *testing.T) {
	steps := []v1beta1.Step{
		{
			Name: "child",
			StepOptions: v1beta1.StepOptions{
				Extends: &v1beta1.StepReference{Name: "nonexistent"},
			},
		},
	}

	_, err := resolveExtends(steps)
	assert.ErrorContains(t, err, "nonexistent")
}

func TestResolveExtends_CircularExtends(t *testing.T) {
	steps := []v1beta1.Step{
		{
			Name: "a",
			StepOptions: v1beta1.StepOptions{
				Extends: &v1beta1.StepReference{Name: "b"},
			},
		},
		{
			Name: "b",
			StepOptions: v1beta1.StepOptions{
				Extends: &v1beta1.StepReference{Name: "a"},
			},
		},
	}

	_, err := resolveExtends(steps)
	assert.ErrorContains(t, err, "circular")
}

func strPtr(s string) *string { return &s }
