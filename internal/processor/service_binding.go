package processor

import (
	"fmt"
	"strings"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithServiceBinding() ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		return &ServiceBinding{
			dependsOn: spec.DependsOn,
		}
	}
}

type ServiceBinding struct {
	dependsOn []v1beta1.TaskDependency
}

func (s *ServiceBinding) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		for _, ref := range s.dependsOn {
			depCtx, ok := ctx.Tasks[ref.Name]
			if !ok {
				return ctx, fmt.Errorf("unable to resolve service binding: %s", ref.Name)
			}

			if len(depCtx.Service.NetIP) == 0 {
				continue
			}

			ctx.Build.State = ctx.Build.State.AddExtraHost(ref.Name, depCtx.Service.NetIP)
			envName := strings.ToUpper(strings.Replace(ref.Name, "-", "_", -1))
			ctx.Build.State = ctx.Build.State.AddEnv(fmt.Sprintf("SERVICE_%s", envName), depCtx.Service.NetIP.String())
		}

		return next(ctx)
	}, nil
}
