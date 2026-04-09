package pipeline

import (
	"encoding/json"
	"fmt"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func resolveExtends(steps []v1beta1.Step) ([]v1beta1.Step, error) {
	stepMap := make(map[string]v1beta1.Step, len(steps))
	for _, step := range steps {
		stepMap[step.Name] = step
	}

	resolved := make([]v1beta1.Step, len(steps))
	for i, step := range steps {
		s, err := resolveStep(step.Name, stepMap, make(map[string]bool))
		if err != nil {
			return nil, err
		}
		resolved[i] = s
	}

	return resolved, nil
}

func resolveStep(name string, stepMap map[string]v1beta1.Step, visiting map[string]bool) (v1beta1.Step, error) {
	step, ok := stepMap[name]
	if !ok {
		return v1beta1.Step{}, fmt.Errorf("unknown step %q", name)
	}

	if len(step.Extends) == 0 {
		return step, nil
	}

	if visiting[name] {
		return v1beta1.Step{}, fmt.Errorf("circular extend detected involving step %q", name)
	}

	visiting[name] = true
	defer delete(visiting, name)
	var mergedStep v1beta1.Step

	for _, extend := range step.Extends {
		template, err := resolveStep(extend.Name, stepMap, visiting)
		if err != nil {
			return v1beta1.Step{}, fmt.Errorf("step %q extends %q: %w", name, extend.Name, err)
		}

		base, err := json.Marshal(template)
		if err != nil {
			return v1beta1.Step{}, fmt.Errorf("failed to marshal template step %q: %w", template.Name, err)
		}

		overlay := step
		overlay.Extends = nil
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
