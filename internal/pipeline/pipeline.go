package pipeline

import (
	"errors"
	"fmt"
	"slices"

	"github.com/raffis/rageta/internal/processor"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

type pipeline struct {
	name       string
	id         string
	entrypoint string
	tasks      []*pipelineTask
}

func (p *pipeline) Name() string {
	return p.name
}

func (p *pipeline) ID() string {
	return p.id
}

func (p *pipeline) Task(name string) (processor.Task, error) {
	for _, task := range p.tasks {
		if task.name == name {
			return task, nil
		}
	}

	return nil, fmt.Errorf("no such task: %s", name)
}

func (p *pipeline) TasksByLabels(labels map[string]string) []processor.Task {
	var tasks []processor.Task
	for _, task := range p.tasks {
		if matchLabels(task.labels, labels) {
			tasks = append(tasks, task)
		}
	}
	return tasks
}

func specLabels(spec v1beta1.Task) map[string]string {
	labels := make(map[string]string, len(spec.Labels))
	for _, l := range spec.Labels {
		labels[l.Name] = l.Value
	}
	return labels
}

func matchLabels(taskLabels map[string]string, selector map[string]string) bool {
	for k, v := range selector {
		if taskLabels[k] != v {
			return false
		}
	}
	return true
}

func (p *pipeline) DependantTasks(name string) []processor.Task {
	var tasks []processor.Task
	for _, task := range p.tasks {
		for _, ref := range task.dependsOn {
			if name == ref {
				tasks = append(tasks, task)
			}
		}
	}

	return tasks
}

func (p *pipeline) TaskDependencies(name string) []string {
	for _, task := range p.tasks {
		if task.name == name {
			return task.dependsOn
		}
	}
	return nil
}

func (p *pipeline) withTask(name string, dependsOn []string, labels map[string]string, processors []processor.Bootstraper) error {
	if slices.ContainsFunc(p.tasks, func(s *pipelineTask) bool {
		return s.name == name
	}) {
		return fmt.Errorf("duplicate task: %s", name)
	}

	p.tasks = append(p.tasks, &pipelineTask{
		name:       name,
		processors: processors,
		pipeline:   p,
		dependsOn:  dependsOn,
		labels:     labels,
	})

	return nil
}

func (p *pipeline) EntrypointName() (string, error) {
	if p.entrypoint == "" {
		if len(p.tasks) == 0 {
			return "", errors.New("no tasks defined")
		}

		return p.tasks[0].name, nil
	}

	return p.entrypoint, nil
}

func (p *pipeline) Entrypoint(name string) (processor.Next, error) {
	if name == "" {
		name = p.entrypoint
	}

	if name != "" {
		task, err := p.Task(name)
		if err != nil {
			return nil, fmt.Errorf("entrypoint not found: %w", err)
		}

		return task.Entrypoint()
	}

	return processor.Chain(p, p.tasks[0].processors...)
}
