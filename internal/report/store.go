package report

import (
	"sort"
	"sync"

	"github.com/raffis/rageta/internal/processor"
)

type stepResult struct {
	stepName string
	result   *processor.StepResult
}

type Store struct {
	steps []stepResult
	mu    sync.Mutex
}

func (s *Store) Add(stepName string, result *processor.StepResult) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for k, v := range s.steps {
		if v.stepName == stepName {
			s.steps[k] = stepResult{
				stepName: stepName,
				result:   result,
			}

			return
		}

	}

	s.steps = append(s.steps, stepResult{
		stepName: stepName,
		result:   result,
	})
}

func (s *Store) Ordered() []stepResult {
	sort.Slice(s.steps, func(i, j int) bool {
		if s.steps[i].result == nil {
			return false
		}

		return s.steps[i].result.StartedAt.Before(s.steps[j].result.StartedAt)
	})

	return s.steps
}
