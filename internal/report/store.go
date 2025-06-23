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
	result   processor.StepContext
}

type store struct {
	steps []stepResult
	mu    sync.Mutex
}

func (s *store) Add(stepName string, ctx processor.StepContext) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for k, v := range s.steps {
		if v.stepName == stepName {
			s.steps[k].result = ctx

			return
		}
	}

	s.steps = append(s.steps, stepResult{
		stepName: stepName,
		result:   ctx,
	})
}

func (s *store) Ordered() []stepResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	sort.Slice(s.steps, func(i, j int) bool {
		var iTags, jTags []string
		for _, tag := range s.steps[i].result.Tags() {
			iTags = append(iTags, fmt.Sprintf("%s:%s", tag.Key, tag.Value))
		}
		for _, tag := range s.steps[j].result.Tags() {
			jTags = append(jTags, fmt.Sprintf("%s:%s", tag.Key, tag.Value))
		}

		iTagsKey := strings.Join(iTags, "-")
		jTagsKey := strings.Join(jTags, "-")

		if iTagsKey == jTagsKey {
			return s.steps[i].result.StartedAt.Before(s.steps[j].result.StartedAt)
		}

		return iTagsKey < jTagsKey
	})

	return s.steps
}
