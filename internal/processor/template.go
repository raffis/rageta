package processor

import (
	"fmt"
	"path/filepath"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithTemplate(template v1beta1.Template) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		return &Template{
			globalTemplate: template,
			stepTemplate:   spec.Template,
		}
	}
}

type Template struct {
	globalTemplate v1beta1.Template
	stepTemplate   *v1beta1.Template
}

func (s *Template) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		originTemplate := ctx.Template

		if ctx.Template == nil {
			ctx.Template = &s.globalTemplate
		}

		if s.stepTemplate != nil {
			if err := mergeTemplate(ctx.Template, s.stepTemplate); err != nil {
				return ctx, err
			}
		}

		ctx, err := next(ctx)
		ctx.Template = originTemplate
		return ctx, err
	}, nil
}

func mergeTemplate(to *v1beta1.Template, from *v1beta1.Template) error {
	if len(to.Args) == 0 {
		to.Args = from.Args
	}
	if len(to.Command) == 0 {
		to.Command = from.Command
	}

	if to.WorkingDir == "" {
		to.WorkingDir = from.WorkingDir
	}

	if to.Image == "" {
		to.Image = from.Image
	}

	if to.Uid == nil {
		to.Uid = from.Uid
	}

	if to.Guid == nil {
		to.Guid = from.Guid
	}

	for _, templateVol := range from.VolumeMounts {
		hasVolume := false
		for _, containerVol := range to.VolumeMounts {
			if templateVol.Name == containerVol.Name {
				hasVolume = true
				break
			}
		}

		if !hasVolume {
			hostPath, err := filepath.Abs(templateVol.HostPath)
			if err != nil {
				return fmt.Errorf("failed to get absolute path: %w", err)
			}

			to.VolumeMounts = append(to.VolumeMounts, v1beta1.VolumeMount{
				Name:      templateVol.Name,
				HostPath:  hostPath,
				MountPath: templateVol.MountPath,
			})
		}
	}

	return nil
}
