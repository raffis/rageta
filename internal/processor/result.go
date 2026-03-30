package processor

import (
	"crypto/sha1"
	"fmt"
	"time"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithResult() ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		return &Result{
			stepName: spec.Name,
		}
	}
}

type Result struct {
	stepName string
}

type stepError struct {
	parent         error
	stepName       string
	uniqueStepName string
	context        StepContext
}

func (e *stepError) Error() string {
	return fmt.Sprintf("step %s failed: %s", e.stepName, e.parent.Error())
}

func (e *stepError) Unwrap() error {
	return e.parent
}

func (e *stepError) StepName() string {
	return e.stepName
}

func (e *stepError) Context() StepContext {
	return e.context
}

func (s *Result) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		ctx.StartedAt = time.Now()

		ctx.uniqueName = s.stepName
		if ctx.namespace != "" {
			ctx.uniqueName = fmt.Sprintf("%s-%s", ctx.namespace, s.stepName)
		}

		hasher := sha1.New()
		hasher.Write([]byte(ctx.uniqueName))
		b := hasher.Sum(nil)
		ctx.uniqueID = fmt.Sprintf("%x", b)

		ctx, err := next(ctx)
		ctx.EndedAt = time.Now()

		if err != nil {
			err = &stepError{
				parent:   err,
				stepName: s.stepName,
				//uniqueStepName: SuffixName(s.stepName, ctx.NamePrefix),
				context: ctx,
			}
			ctx.Error = err
		} else {
			ctx.Error = nil
		}

		ctx.Steps[s.stepName] = &ctx
		ctx.uniqueName = ""
		ctx.namespace = ""
		ctx.uniqueID = ""

		return ctx, err
	}, nil
}
