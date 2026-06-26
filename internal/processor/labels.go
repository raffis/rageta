package processor

import (
	"sync"

	"github.com/raffis/rageta/internal/styles"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithLabels(globalLabels []Label) ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		if len(globalLabels) == 0 && len(spec.Labels) == 0 {
			return nil
		}

		return &Labels{
			labels:       spec.Labels,
			globalLabels: globalLabels,
		}
	}
}

type Labels struct {
	labels       []v1beta1.Label
	globalLabels []Label
}

type LabelsContext struct {
	labels []Label
}

func (s *Labels) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	var labels []Label
	for _, label := range s.labels {
		labels = append(labels, Label{
			Key:      label.Name,
			Value:    label.Value,
			HEXColor: label.HEXColor,
		})
	}

	labels = append(labels, s.globalLabels...)

	return func(ctx TaskContext) (TaskContext, error) {
		//originLabels := slices.Clone(ctx.Labels.labels)

		for _, label := range labels {
			ctx.Labels.Add(label)
		}

		ctx, err := next(ctx)
		//ctx.Labels.labels = originLabels

		return ctx, err
	}, nil
}

type Label struct {
	Key      string
	Value    string
	HEXColor string
}

func (t LabelsContext) Labels() []Label {
	return t.labels
}

func (t LabelsContext) Has(key string) bool {
	for _, v := range t.labels {
		if v.Key == key {
			return true
		}
	}

	return false
}

var (
	labelColors = make(map[Label]string)
	labelMutex  = sync.Mutex{}
)

func (t *LabelsContext) Match(selector map[string]string) bool {
	for k, v := range selector {
		found := false
		for _, label := range t.labels {
			if label.Key == k && label.Value == v {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func (t *LabelsContext) Add(label Label) {
	labelMutex.Lock()
	defer labelMutex.Unlock()

	if v, ok := labelColors[label]; ok {
		label.HEXColor = v
	} else {
		if label.HEXColor == "" {
			color := styles.RandHEXColor(0, 255)
			labelColors[label] = color
			label.HEXColor = color
		} else {
			labelColors[label] = label.HEXColor
		}
	}

	for i, v := range t.labels {
		if v.Key == label.Key {
			t.labels[i] = label
			return
		}
	}

	t.labels = append(t.labels, label)
}
