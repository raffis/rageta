package processor

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/raffis/rageta/internal/substitute"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"

	"github.com/moby/buildkit/client/llb"
)

func WithSources() ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		if spec.Sources == nil {
			return nil
		}
		return &Sources{
			sources: spec.Sources,
		}
	}
}

type Sources struct {
	sources []v1beta1.Source
}

var ErrUnknownSourceType = errors.New("unknown source type")

func (s *Sources) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		sources := make([]v1beta1.Source, len(s.sources))
		subst := []any{}

		for i := range sources {
			sources[i] = *s.sources[i].DeepCopy()
			switch {
			case sources[i].Local != nil:
				subst = append(subst, &sources[i].Local.Path, &sources[i].Local.To)
			case sources[i].Task != nil:
				subst = append(subst, &sources[i].Task.Name, &sources[i].Task.Path, &sources[i].Task.To)
			case sources[i].Tasks != nil:
				subst = append(subst, &sources[i].Tasks.Path, &sources[i].Tasks.To)
			case sources[i].Context != nil:
				subst = append(subst, &sources[i].Context.Path, &sources[i].Context.To)
			default:
				return ctx, ErrUnknownSourceType
			}
		}
		if err := substitute.Substitute(ctx.ToV1Beta1(), subst...); err != nil {
			return ctx, err
		}

		for _, source := range sources {
			switch {
			case source.Local != nil:
				srcPath := source.Local.Path
				if srcPath == "" {
					srcPath = "."
				}
				copyTo := source.Local.To
				if copyTo == "" {
					copyTo = "."
				}

				var contextSrc llb.State
				var copyFrom string
				if strings.ContainsAny(srcPath, "*?[") {
					contextSrc = llb.Local("context", llb.IncludePatterns([]string{srcPath}))
					copyFrom = globBaseDir(srcPath)
				} else {
					contextSrc = llb.Local("context")
					copyFrom = srcPath
				}

				ctx.Build.State = ctx.Build.State.File(
					llb.Copy(contextSrc, copyFrom, copyTo, &llb.CopyInfo{
						CreateDestPath:      true,
						CopyDirContentsOnly: true,
					}),
					llb.WithCustomNamef("copy CONTEXT:%s → %s", srcPath, copyTo),
				)

			case source.Task != nil:
				stepCtx, ok := ctx.Tasks[source.Task.Name]
				if !ok {
					return ctx, fmt.Errorf("source step %q dependency not found", source.Task.Name)
				}

				srcPath := source.Task.Path
				if srcPath == "" {
					srcPath = "."
				}
				dst := source.Task.To
				if dst == "" {
					dst = "."
				}

				copyInfo := &llb.CopyInfo{CreateDestPath: true, CopyDirContentsOnly: true}

				ctx.Build.State = ctx.Build.State.File(
					llb.Copy(stepCtx.Build.State, srcPath, dst, copyInfo),
					llb.WithCustomNamef("copy %s:%s → %s", source.Task.Name, srcPath, dst),
				)

			case source.Tasks != nil:
				srcPath := source.Tasks.Path
				if srcPath == "" {
					srcPath = "."
				}
				dst := source.Tasks.To
				if dst == "" {
					dst = "."
				}

				copyInfo := &llb.CopyInfo{CreateDestPath: true, CopyDirContentsOnly: true}

				var matched int
				for name, stepCtx := range ctx.Tasks {
					if !stepCtx.Labels.Match(source.Tasks.MatchLabels) {
						continue
					}

					dstPath := path.Join(dst, stepCtx.uniqueName)

					matched++
					ctx.Build.State = ctx.Build.State.File(
						llb.Copy(stepCtx.Build.State, srcPath, dstPath, copyInfo),
						llb.WithCustomNamef("copy %s:%s → %s", name, srcPath, dstPath),
					)
				}
				if matched == 0 {
					return ctx, fmt.Errorf("no tasks matched label selector %v", source.Tasks.MatchLabels)
				}

			case source.Context != nil:
				if ctx.Build.ContextState == nil {
					return ctx, fmt.Errorf("context source requires running inside an inherit")
				}

				srcPath := source.Context.Path
				if srcPath == "" {
					srcPath = "."
				}
				dst := source.Context.To
				if dst == "" {
					dst = "."
				}

				copyInfo := &llb.CopyInfo{CreateDestPath: true, CopyDirContentsOnly: true}

				ctx.Build.State = ctx.Build.State.File(
					llb.Copy(*ctx.Build.ContextState, srcPath, dst, copyInfo),
					llb.WithCustomNamef("copy INHERIT:%s → %s", srcPath, dst),
				)

			default:
				return ctx, ErrUnknownSourceType
			}
		}

		return next(ctx)
	}, nil
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
