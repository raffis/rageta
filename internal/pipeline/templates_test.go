package pipeline

import (
	"testing"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveTemplates_NoTemplates(t *testing.T) {
	steps := []v1beta1.Step{
		{Name: "a", Run: &v1beta1.RunStep{Container: v1beta1.Container{Image: "alpine"}}},
		{Name: "b", Run: &v1beta1.RunStep{Container: v1beta1.Container{Image: "ubuntu"}}},
	}

	resolved, err := resolveTemplates(steps)
	require.NoError(t, err)
	assert.Equal(t, steps, resolved)
}

func TestResolveTemplates_SimpleTemplates(t *testing.T) {
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

	Templatesing := v1beta1.Step{
		Name: "child",
		StepOptions: v1beta1.StepOptions{
			Templates: []v1beta1.LocalReference{{Name: "base"}},
			Env: []v1beta1.EnvVar{
				{Name: "EXTRA", Value: strPtr("val")},
			},
		},
	}

	resolved, err := resolveTemplates([]v1beta1.Step{template, Templatesing})
	require.NoError(t, err)
	require.Len(t, resolved, 2)

	child := resolved[1]
	assert.Equal(t, "child", child.Name)
	assert.Nil(t, child.Templates, "Templates should be cleared after resolution")
	assert.Equal(t, "alpine", child.Run.Image)
	assert.Equal(t, "echo base", child.Run.Script)
	assert.True(t, child.AllowFailure)
	// Env from the Templatesing step overrides (merge patch replaces arrays)
	assert.Equal(t, []v1beta1.EnvVar{{Name: "EXTRA", Value: strPtr("val")}}, child.Env)
}

func TestResolveTemplates_OverrideFields(t *testing.T) {
	template := v1beta1.Step{
		Name: "base",
		Run: &v1beta1.RunStep{
			Container: v1beta1.Container{Image: "alpine", Script: "echo template"},
		},
	}

	Templatesing := v1beta1.Step{
		Name: "child",
		StepOptions: v1beta1.StepOptions{
			Templates: []v1beta1.LocalReference{{Name: "base"}},
		},
		Run: &v1beta1.RunStep{
			Container: v1beta1.Container{Image: "ubuntu"},
		},
	}

	resolved, err := resolveTemplates([]v1beta1.Step{template, Templatesing})
	require.NoError(t, err)

	child := resolved[1]
	assert.Equal(t, "ubuntu", child.Run.Image)
	// Script from template is kept since Templatesing step did not set it
	assert.Equal(t, "echo template", child.Run.Script)
}

func TestResolveTemplates_ChainedTemplates(t *testing.T) {
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
			Templates: []v1beta1.LocalReference{{Name: "grandparent"}},
			Env:       []v1beta1.EnvVar{{Name: "LEVEL", Value: strPtr("parent")}},
		},
	}

	child := v1beta1.Step{
		Name: "child",
		StepOptions: v1beta1.StepOptions{
			Templates: []v1beta1.LocalReference{{Name: "parent"}},
			Env:       []v1beta1.EnvVar{{Name: "LEVEL", Value: strPtr("child")}},
		},
	}

	resolved, err := resolveTemplates([]v1beta1.Step{grandparent, parent, child})
	require.NoError(t, err)

	childStep := resolved[2]
	assert.Equal(t, "child", childStep.Name)
	assert.Nil(t, childStep.Templates)
	assert.Equal(t, "alpine", childStep.Run.Image)
	assert.True(t, childStep.AllowFailure)
	assert.Equal(t, []v1beta1.EnvVar{{Name: "LEVEL", Value: strPtr("child")}}, childStep.Env)
}

func TestResolveTemplates_UnknownTemplate(t *testing.T) {
	steps := []v1beta1.Step{
		{
			Name: "child",
			StepOptions: v1beta1.StepOptions{
				Templates: []v1beta1.LocalReference{{Name: "nonexistent"}},
			},
		},
	}

	_, err := resolveTemplates(steps)
	assert.ErrorContains(t, err, "nonexistent")
}

func TestResolveTemplates_CircularTemplates(t *testing.T) {
	steps := []v1beta1.Step{
		{
			Name: "a",
			StepOptions: v1beta1.StepOptions{
				Templates: []v1beta1.LocalReference{{Name: "b"}},
			},
		},
		{
			Name: "b",
			StepOptions: v1beta1.StepOptions{
				Templates: []v1beta1.LocalReference{{Name: "a"}},
			},
		},
	}

	_, err := resolveTemplates(steps)
	assert.ErrorContains(t, err, "circular")
}

func strPtr(s string) *string { return &s }
