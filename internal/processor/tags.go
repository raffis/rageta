package processor

import (
	"slices"
	"sync"

	"github.com/raffis/rageta/internal/styles"
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

type TagsContext struct {
	tags []Tag
}

func (s *Tags) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	var tags []Tag
	for _, tag := range s.tags {
		tags = append(tags, Tag{
			Key:      tag.Name,
			Value:    tag.Value,
			HEXColor: tag.HEXColor,
		})
	}

	tags = append(tags, s.globalTags...)

	return func(ctx StepContext) (StepContext, error) {
		originTags := slices.Clone(ctx.Tags.tags)

		for _, tag := range tags {
			ctx.Tags.Add(tag)
		}

		ctx, err := next(ctx)
		ctx.Tags.tags = originTags
		return ctx, err
	}, nil
}

type Tag struct {
	Key      string
	Value    string
	HEXColor string
}

func (t TagsContext) Tags() []Tag {
	return t.tags
}

func (t TagsContext) Has(key string) bool {
	for _, v := range t.tags {
		if v.Key == key {
			return true
		}
	}

	return false
}

var (
	tagColors = make(map[Tag]string)
	tagMutex  = sync.Mutex{}
)

func (t *TagsContext) Add(tag Tag) {
	tagMutex.Lock()
	defer tagMutex.Unlock()

	if v, ok := tagColors[tag]; ok {
		tag.HEXColor = v
	} else {
		if tag.HEXColor == "" {
			color := styles.RandHEXColor(0, 255)
			tagColors[tag] = color
			tag.HEXColor = color
		} else {
			tagColors[tag] = tag.HEXColor
		}
	}

	for i, v := range t.tags {
		if v.Key == tag.Key {
			t.tags[i] = tag
			return
		}
	}

	t.tags = append(t.tags, tag)
}
