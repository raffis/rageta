package processor

import (
	"errors"
)

func AbortOnError(err error) bool {
	if err == nil {
		return false
	}

	var abortPipeline ErrorIsAbortable
	if errors.As(err, &abortPipeline) {
		return abortPipeline.AbortOnError()
	}

	return true
}

func ErrorResult(err error) string {
	if err == nil {
		return "success"
	}

	var result ErrorGetResult
	if errors.As(err, &result) {
		return result.Result()
	}

	return "error"
}

type pipelineError struct {
	message      string
	result       string
	abortOnError bool
}

func (e *pipelineError) Error() string {
	return e.message
}

func (e *pipelineError) AbortOnError() bool {
	return e.abortOnError
}

func (e *pipelineError) Result() string {
	return e.result
}

type ErrorIsAbortable interface {
	AbortOnError() bool
}

type ErrorGetResult interface {
	Result() string
}

type StepError interface {
	StepName() string
	Context() StepContext
}

type ErrorContainer interface {
	ExitCode
	Image() string
	ContainerName() string
}

type ExitCode interface {
	ExitCode() int
}
