package pipeline

import (
	"errors"
	"fmt"
	"slices"

	"github.com/raffis/rageta/internal/processor"
)

type pipeline struct {
	name       string
	id         string
	entrypoint string
	steps      []*pipelineTask
}

type pipelineTask struct {
	processors []processor.Bootstraper
	name       string
	pipeline   *pipeline
	dependsOn  []string
}

func (p *pipelineTask) Processors() []processor.Bootstraper {
	return p.processors
}

func (p *pipelineTask) Name() string {
	return p.name
}

func (p *pipelineTask) Entrypoint() (processor.Next, error) {
	return processor.Chain(p.pipeline, p.processors...)
}

func (p *pipeline) Name() string {
	return p.name
}

func (p *pipeline) ID() string {
	return p.id
}

func (p *pipeline) Task(name string) (processor.Task, error) {
	for _, step := range p.steps {
		if step.name == name {
			return step, nil
		}
	}

	return nil, fmt.Errorf("no such step exists: %s", name)
}

func (p *pipeline) DependantTasks(name string) []processor.Task {
	var steps []processor.Task
	for _, step := range p.steps {
		for _, ref := range step.dependsOn {
			if name == ref {
				steps = append(steps, step)
			}
		}
	}

	return steps
}

func (p *pipeline) TaskDependencies(name string) []string {
	for _, step := range p.steps {
		if step.name == name {
			return step.dependsOn
		}
	}
	return nil
}

func (p *pipeline) withTask(name string, dependsOn []string, processors []processor.Bootstraper) error {
	if slices.ContainsFunc(p.steps, func(s *pipelineTask) bool {
		return s.name == name
	}) {
		return fmt.Errorf("duplicate step: %s", name)
	}

	p.steps = append(p.steps, &pipelineTask{
		name:       name,
		processors: processors,
		pipeline:   p,
		dependsOn:  dependsOn,
	})

	return nil
}

func (p *pipeline) EntrypointName() (string, error) {
	if p.entrypoint == "" {
		if len(p.steps) == 0 {
			return "", errors.New("no steps defined")
		}

		return p.steps[0].name, nil
	}

	return p.entrypoint, nil
}

func (p *pipeline) Entrypoint(name string) (processor.Next, error) {
	if name == "" {
		name = p.entrypoint
	}

	if name != "" {
		step, err := p.Task(name)
		if err != nil {
			return nil, fmt.Errorf("entrypoint not found: %w", err)
		}

		return step.Entrypoint()
	}

	return processor.Chain(p, p.steps[0].processors...)
}
