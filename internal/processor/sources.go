package processor

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/raffis/rageta/internal/substitute"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"

	"github.com/moby/buildkit/client/llb"
)

func WithSources() ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
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

func (s *Sources) Bootstrap(_ Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		subst := []any{}

		for i := range s.sources {
			switch {
			case s.sources[i].Local != nil:
				subst = append(subst, &s.sources[i].Local.Path, &s.sources[i].Local.To)
			case s.sources[i].Step != nil:
				subst = append(subst, &s.sources[i].Step.Name, &s.sources[i].Step.Path, &s.sources[i].Step.To)
			default:
				return ctx, errors.New("no source type given")
			}
		}
		if err := substitute.Substitute(ctx.ToV1Beta1(), subst...); err != nil {
			return ctx, err
		}

		for _, source := range s.sources {
			switch {
			case source.Local != nil:
				srcPath := source.Local.Path
				if srcPath == "" {
					srcPath = "."
				}
				copyTo := source.Local.To
				if copyTo == "" {
					copyTo = srcPath
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

			case source.Step != nil:
				stepCtx, ok := ctx.Steps[source.Step.Name]
				if !ok {
					return ctx, fmt.Errorf("source step %q dependency not found", source.Step.Name)
				}

				srcPath := source.Step.Path
				if srcPath == "" {
					srcPath = "/"
				}
				dst := source.Step.To
				if dst == "" {
					dst = srcPath
				}

				copyInfo := &llb.CopyInfo{CreateDestPath: true}
				if srcPath == "/" || strings.HasSuffix(srcPath, "/") {
					copyInfo.CopyDirContentsOnly = true
				}

				ctx.Build.State = ctx.Build.State.File(
					llb.Copy(stepCtx.Build.State, srcPath, dst, copyInfo),
					llb.WithCustomNamef("copy %s:%s → %s", source.Step.Name, srcPath, dst),
				)
			default:
				return ctx, errors.New("no source type given")
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
