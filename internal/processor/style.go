package processor

import (
	"charm.land/lipgloss/v2"
	"github.com/raffis/rageta/internal/styles"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithStyle() ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		return &Style{}
	}
}

type Style struct {
}

type StyleContext struct {
	Style lipgloss.Style
}

func (s *Style) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		ctx.Style.Style = lipgloss.NewStyle().Foreground(styles.RandAdaptiveColor())
		return next(ctx)
	}, nil
}
