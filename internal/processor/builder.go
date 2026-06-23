package processor

import (
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

type ProcessorBuilder func(spec *v1beta1.Task) Bootstraper

func Builder(spec *v1beta1.Task, builders ...ProcessorBuilder) []Bootstraper {
	var result []Bootstraper
	for _, builder := range builders {
		processor := builder(spec)
		if processor != nil {
			result = append(result, processor)
		}
	}

	return result
}
