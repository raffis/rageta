package pipeline

import (
	"errors"
	"fmt"
	"slices"

	"github.com/raffis/rageta/internal/processor"
)

type pipeline struct {
	name        string
	id          string
	defaultTask string
	tasks       []*pipelineTask
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

// dependsOnRef records one dependsOn edge together with the modifiers that
// were declared on it, e.g. whether it should await a matrix in full instead
// of running once per matrix combination.
type dependsOnRef struct {
	name        string
	awaitMatrix bool
}

func (p *pipeline) TaskDependencies(name string) []string {
	for _, task := range p.tasks {
		if task.name == name {
			names := make([]string, 0, len(task.dependsOn))
			for _, ref := range task.dependsOn {
				names = append(names, ref.name)
			}
			return names
		}
	}
	return nil
}

// ChildTasks returns tasks that depend on name and should be launched once
// per matrix combination of name (the default dependsOn behavior).
func (p *pipeline) ChildTasks(name string) []processor.Task {
	var tasks []processor.Task
	for _, task := range p.tasks {
		for _, ref := range task.dependsOn {
			if ref.name == name && !ref.awaitMatrix {
				tasks = append(tasks, task)
				break
			}
		}
	}

	return tasks
}

// AwaitMatrixChildren returns tasks that depend on name with awaitMatrix set,
// i.e. tasks that should be launched exactly once after every matrix
// combination of name has finished.
func (p *pipeline) AwaitMatrixChildren(name string) []processor.Task {
	var tasks []processor.Task
	for _, task := range p.tasks {
		for _, ref := range task.dependsOn {
			if ref.name == name && ref.awaitMatrix {
				tasks = append(tasks, task)
				break
			}
		}
	}

	return tasks
}

func (p *pipeline) withTask(name string, dependsOn []dependsOnRef, processors []processor.Bootstraper) error {
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
	})

	return nil
}

func (p *pipeline) Entrypoint(name string) (processor.Next, error) {
	if name == "" && p.defaultTask == "" {
		return nil, errors.New("task target expected")
	}

	if name == "" {
		name = p.defaultTask
	}

	for _, task := range p.tasks {
		if task.name == name {
			return task.Entrypoint()
		}
	}

	return nil, errors.New("task target no found")
}
