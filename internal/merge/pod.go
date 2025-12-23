package merge

import (
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/util/strategicpatch"
)

func Pod(base, patch corev1.Pod) (*corev1.Pod, error) {
	baseBytes, err := json.Marshal(base)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON for base: %w", err)
	}

	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON for patch: %w", err)
	}

	jsonResult, err := strategicpatch.StrategicMergePatch(baseBytes, patchBytes, corev1.Pod{})
	if err != nil {
		return nil, fmt.Errorf("failed to generate merge patch: %w", err)
	}

	var patchResult corev1.Pod
	if err := json.Unmarshal(jsonResult, &patchResult); err != nil {
		return nil, fmt.Errorf("failed to unmarshal merged pod: %w", err)
	}

	return &patchResult, nil
}
