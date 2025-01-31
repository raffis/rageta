package pipeline

import (
	"fmt"
	"slices"

	"github.com/raffis/rageta/internal/processor"
)

type pipeline struct {
	name       string
	id         string
	entrypoint string
	steps      []*pipelineStep
}

type pipelineStep struct {
	processors []processor.Bootstraper
	name       string
	pipeline   *pipeline
}

func (p *pipelineStep) Processors() []processor.Bootstraper {
	return p.processors
}

func (p *pipelineStep) Entrypoint() (processor.Next, error) {
	return processor.Chain(p.pipeline, p.processors...)
}

func (p *pipeline) Name() string {
	return p.name
}

func (p *pipeline) ID() string {
	return p.id
}

func (p *pipeline) Step(name string) (processor.Step, error) {
	for _, step := range p.steps {
		if step.name == name {
			return step, nil
		}
	}

	return nil, fmt.Errorf("no such step found: %s", name)
}

func (p *pipeline) withStep(name string, processors []processor.Bootstraper) error {
	if slices.ContainsFunc(p.steps, func(s *pipelineStep) bool {
		return s.name == name
	}) {
		return fmt.Errorf("step already exists: %s", name)
	}

	p.steps = append(p.steps, &pipelineStep{
		name:       name,
		processors: processors,
		pipeline:   p,
	})

	return nil
}

func (p *pipeline) Entrypoint(name string) (processor.Next, error) {
	if name == "" {
		name = p.entrypoint
	}

	if name != "" {
		step, err := p.Step(name)
		if err != nil {
			return nil, fmt.Errorf("entrypoint not found: %w", err)
		}

		return step.Entrypoint()
	}

	return processor.Chain(p, p.steps[0].processors...)
}
