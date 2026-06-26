package report

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/raffis/rageta/internal/processor"
)

type stepResult struct {
	stepName string
	result   processor.TaskContext
}

type store struct {
	steps []stepResult
	mu    sync.Mutex
}

func (s *store) Add(stepName string, ctx processor.TaskContext) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.steps = append(s.steps, stepResult{
		stepName: stepName,
		result:   ctx,
	})
}

func (s *store) Ordered() []stepResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	sort.Slice(s.steps, func(i, j int) bool {
		var iLabels, jLabels []string
		for _, tag := range s.steps[i].result.Labels.Labels() {
			iLabels = append(iLabels, fmt.Sprintf("%s:%s", tag.Key, tag.Value))
		}
		for _, tag := range s.steps[j].result.Labels.Labels() {
			jLabels = append(jLabels, fmt.Sprintf("%s:%s", tag.Key, tag.Value))
		}

		iLabelsKey := strings.Join(iLabels, "-")
		jLabelsKey := strings.Join(jLabels, "-")

		if iLabelsKey == jLabelsKey {
			return s.steps[i].result.StartedAt.Before(s.steps[j].result.StartedAt)
		}

		return iLabelsKey < jLabelsKey
	})

	return s.steps
}
