package processor

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/raffis/rageta/internal/substitute"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"

	"github.com/moby/buildkit/client/llb"
)

func WithCopy() ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		if spec.Copy == nil {
			return nil
		}

		return &Copy{
			sources: spec.Copy,
		}
	}
}

type Copy struct {
	sources []v1beta1.Copy
}

func (s *Copy) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		sources := make([]v1beta1.Copy, len(s.sources))
		subst := []any{}

		for i := range sources {
			sources[i] = *s.sources[i].DeepCopy()
			subst = append(subst, &sources[i].Path, &sources[i].To)
		}

		if err := substitute.Substitute(ctx.ToV1Beta1(), subst...); err != nil {
			return ctx, err
		}

		for _, source := range sources {
			srcPath := source.Path
			if srcPath == "" {
				srcPath = "."
			}
			copyTo := source.To
			if copyTo == "" {
				copyTo = "."
			}

			copyInfo := &llb.CopyInfo{
				CreateDestPath:      true,
				CopyDirContentsOnly: true,
			}

			if source.Chmod != nil {
				copyInfo.Mode = &llb.ChmodOpt{
					Mode: os.FileMode(*source.Chmod),
				}
			}

			if source.From == nil {
				//var contextSrc llb.State
				var copyFrom string
				/*if strings.ContainsAny(srcPath, "*?[") {
					contextSrc = llb.Local("context", llb.IncludePatterns([]string{srcPath}))
					copyFrom = globBaseDir(srcPath)
				} else {
					contextSrc = llb.Local("context")
					copyFrom = srcPath
				}*/
				copyFrom = srcPath
				ctx.Build.State = ctx.Build.State.File(
					llb.Copy(ctx.Build.ContextState, copyFrom, copyTo, copyInfo),
					llb.WithCustomNamef("copy CONTEXT:%s → %s", srcPath, copyTo),
				)
			} else {
				taskName := *source.From
				copyInfo := &llb.CopyInfo{CreateDestPath: true, CopyDirContentsOnly: true}

				if instances, ok := ctx.TaskGroups[taskName]; ok {
					// Matrix task: fan out and copy from every instance into
					// its own subdirectory, named after its matrix params.
					for _, stepCtx := range instances {
						dst := filepath.Join(copyTo, matrixInstanceDir(stepCtx))

						ctx.Build.State = ctx.Build.State.File(
							llb.Copy(stepCtx.Build.State, srcPath, dst, copyInfo),
							llb.WithCustomNamef("copy %s:%s → %s", stepCtx.UniqueName(), srcPath, dst),
						)
					}

					break
				}

				stepCtx, ok := ctx.Tasks[taskName]
				if !ok {
					return ctx, fmt.Errorf("source step %q dependency not found", taskName)
				}

				ctx.Build.State = ctx.Build.State.File(
					llb.Copy(stepCtx.Build.State, srcPath, copyTo, copyInfo),
					llb.WithCustomNamef("copy %s:%s → %s", taskName, srcPath, copyTo),
				)
			}
		}

		return next(ctx)
	}, nil
}

// matrixInstanceDir derives a stable, human-readable directory name for one
// matrix combination from its params, e.g. {"arch":"amd64","os":"linux"} →
// "arch=amd64,os=linux". Falls back to the instance's unique name if it
// carries no matrix params (defensive; TaskGroups is only ever populated
// with matrix instances).
func matrixInstanceDir(instance *TaskContext) string {
	if len(instance.Matrix.Params) == 0 {
		return instance.UniqueName()
	}

	keys := make([]string, 0, len(instance.Matrix.Params))
	for k := range instance.Matrix.Params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, instance.Matrix.Params[k]))
	}

	return strings.Join(parts, ",")
}

func globBaseDir(pattern string) string {
	parts := strings.Split(filepath.Clean(pattern), string(filepath.Separator))
	var base []string
	for _, p := range parts {
		if strings.ContainsAny(p, "*?[") {
			break
		}
		base = append(base, p)
	}
	if len(base) == 0 {
		return "."
	}
	return filepath.Join(base...)
}
