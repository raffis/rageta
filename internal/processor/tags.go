package processor

import (
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithTags(globalTags []Tag) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if len(globalTags) == 0 && len(spec.Tags) == 0 {
			return nil
		}

		return &Tags{
			tags:       spec.Tags,
			globalTags: globalTags,
		}
	}
}

type Tags struct {
	tags       []v1beta1.Tag
	globalTags []Tag
}

func (s *Tags) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	var tags []Tag
	for _, tag := range s.tags {
		tags = append(tags, Tag{
			Key:  tag.Name,
			Value: tag.Value,
			Color: tag.Color,
		})
	}

	tags = append(tags, s.globalTags...)

	return func(ctx StepContext) (StepContext, error) {
		for _, tag := range tags {
			ctx = ctx.WithTag(tag)
		}

		return next(ctx)
	}, nil
}
