package processor

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"time"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithResult() ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		return &Result{
			taskName: spec.Name,
		}
	}
}

type Result struct {
	taskName string
}

func (s *Result) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		ctx.StartedAt = time.Now()

		ctx.uniqueName = s.taskName
		if ctx.namespace != "" {
			ctx.uniqueName = fmt.Sprintf("%s-%s", ctx.namespace, s.taskName)
		}

		hasher := sha1.New()
		hasher.Write([]byte(ctx.UniqueName()))
		b := hasher.Sum(nil)
		ctx.uniqueID = fmt.Sprintf("%x", b)
		ctx.Tasks[s.taskName] = &ctx

		ctx, err := next(ctx)
		ctx.EndedAt = time.Now()
		ctx.Error = nil

		if err != nil {
			if uw, ok := err.(interface{ Unwrap() []error }); ok {
				err = &multiTaskError{
					parents:  uw.Unwrap(),
					taskName: s.taskName,
					context:  ctx,
					uniqueID: ctx.uniqueID,
				}
			} else {
				err = &taskError{
					parent:   err,
					taskName: s.taskName,
					context:  ctx,
					uniqueID: ctx.uniqueID,
				}
			}

			ctx.Error = err
		}

		ctx.uniqueName = ""
		ctx.namespace = ""
		ctx.uniqueID = ""

		return ctx, err
	}, nil
}

type taskError struct {
	parent   error
	taskName string
	uniqueID string
	context  TaskContext
}

func (e *taskError) Error() string {
	return fmt.Sprintf("task %s failed: %s", e.taskName, e.parent.Error())
}

func (e *taskError) Unwrap() error {
	return e.parent
}

func (e *taskError) TaskName() string {
	return e.taskName
}

func (e *taskError) Context() TaskContext {
	return e.context
}

type multiTaskError struct {
	parents  []error
	taskName string
	uniqueID string
	context  TaskContext
}

func (e *multiTaskError) Error() string {
	return fmt.Sprintf("task %s failed: %s", e.taskName, errors.Join(e.parents...).Error())
}

func (e *multiTaskError) Unwrap() []error {
	return e.parents
}

func (e *multiTaskError) TaskName() string {
	return e.taskName
}

func (e *multiTaskError) Context() TaskContext {
	return e.context
}
