package report

import (
	"sort"
	"sync"

	"github.com/raffis/rageta/internal/processor"
)

type stepResult struct {
	stepName string
	result   processor.StepContext
}

type store struct {
	steps []stepResult
	mu    sync.Mutex
}

func (s *store) Add(stepName string, stepContext processor.StepContext) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for k, v := range s.steps {
		if v.stepName == stepName {
			s.steps[k].result = stepContext

			return
		}
	}

	s.steps = append(s.steps, stepResult{
		stepName: stepName,
		result:   stepContext,
	})
}

func (s *store) Ordered() []stepResult {
	sort.Slice(s.steps, func(i, j int) bool {
		if s.steps[i].result.Error == nil {
			return false
		}

		if s.steps[j].result.Error == nil {
			return false
		}

		return s.steps[i].result.StartedAt.Before(s.steps[j].result.StartedAt)
	})

	return s.steps
}
