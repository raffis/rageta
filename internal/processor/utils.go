package processor

import (
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func refSlice(steps []v1beta1.LocalReference) []string {
	var refs []string
	for _, ref := range steps {
		refs = append(refs, ref.Name)
	}

	return refs
}

func Chain(pipeline Pipeline, s ...Bootstraper) (Next, error) {
	if len(s) == 0 {
		return func(ctx TaskContext) (TaskContext, error) {
			return ctx, nil
		}, nil
	}

	next, err := Chain(pipeline, s[1:]...)
	if err != nil {
		return nil, err
	}

	return s[0].Bootstrap(pipeline, next)
}
