package processor

import (
	"errors"
)

func AbortOnError(err error) bool {
	if err == nil {
		return false
	}

	var abortPipeline IsAbortableError
	if errors.As(err, &abortPipeline) {
		return abortPipeline.AbortOnError()
	}

	return true
}

func ErrorResult(err error) string {
	if err == nil {
		return "success"
	}

	var result ResultError
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

type IsAbortableError interface {
	AbortOnError() bool
}

type ResultError interface {
	Result() string
}

type TaskError interface {
	TaskName() string
	Context() TaskContext
}

type ImageName interface {
	Image() string
}

type ExitCode interface {
	ExitCode() int
}
