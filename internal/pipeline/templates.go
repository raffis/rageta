package pipeline

import (
	"encoding/json"
	"fmt"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func resolveTemplates(steps, templates []v1beta1.Step) ([]v1beta1.Step, error) {
	stepMap := make(map[string]v1beta1.Step, len(steps))
	for _, step := range steps {
		stepMap[step.Name] = step
	}

	templateMap := make(map[string]v1beta1.Step, len(templates))
	for _, step := range templates {
		templateMap[step.Name] = step
	}

	resolved := make([]v1beta1.Step, len(steps))
	for i, step := range steps {
		s, err := resolveStep(step.Name, stepMap, templateMap)
		if err != nil {
			return nil, err
		}
		resolved[i] = s
	}

	return resolved, nil
}

func resolveStep(name string, stepMap, templateMap map[string]v1beta1.Step) (v1beta1.Step, error) {
	step, ok := stepMap[name]
	if !ok {
		return v1beta1.Step{}, fmt.Errorf("unknown step %q", name)
	}

	if len(step.Templates) == 0 {
		return step, nil
	}

	var mergedStep v1beta1.Step

	for _, template := range step.Templates {
		if _, ok := templateMap[template.Name]; !ok {
			return v1beta1.Step{}, fmt.Errorf("template not found: %q", template.Name)
		}

		base, err := json.Marshal(templateMap[template.Name])
		if err != nil {
			return v1beta1.Step{}, fmt.Errorf("failed to marshal template step %q: %w", template.Name, err)
		}

		overlay := step
		overlay.Templates = nil
		patch, err := json.Marshal(overlay)
		if err != nil {
			return v1beta1.Step{}, fmt.Errorf("failed to marshal step %q: %w", name, err)
		}

		merged, err := jsonpatch.MergePatch(base, patch)
		if err != nil {
			return v1beta1.Step{}, fmt.Errorf("failed to merge step %q with template %q: %w", name, template.Name, err)
		}

		if err := json.Unmarshal(merged, &mergedStep); err != nil {
			return v1beta1.Step{}, fmt.Errorf("failed to unmarshal merged step %q: %w", name, err)
		}
	}

	return mergedStep, nil
}
