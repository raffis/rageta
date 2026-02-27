package processor

func AbortOnError(err error) bool {
	if abortable, ok := err.(errorIsAbortable); ok {
		return abortable.AbortOnError()
	}

	if err != nil {
		return true
	}

	return false
}

func ErrorResult(err error) string {
	if abortable, ok := err.(errorResult); ok {
		return abortable.Result()
	}

	if err != nil {
		return "error"
	}

	return "success"
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

type errorIsAbortable interface {
	AbortOnError() bool
}

type errorResult interface {
	Result() string
}
