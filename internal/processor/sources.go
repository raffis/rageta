package processor

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

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
		for _, source := range s.sources {
			srcPath := source.Path
			if srcPath == "" {
				srcPath = "."
			}
			copyTo := source.Path
			if copyTo == "" {
				copyTo = "."
			}

			switch {
			case source.From == nil:
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

			case source.From != nil:
				taskName := *source.From
				stepCtx, ok := ctx.Tasks[taskName]
				if !ok {
					return ctx, fmt.Errorf("source step %q dependency not found", taskName)
				}

				copyInfo := &llb.CopyInfo{CreateDestPath: true, CopyDirContentsOnly: true}

				ctx.Build.State = ctx.Build.State.File(
					llb.Copy(stepCtx.Build.State, srcPath, copyTo, copyInfo),
					llb.WithCustomNamef("copy %s:%s → %s", taskName, srcPath, copyTo),
				)

			/*case source.Tasks != nil:
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
			}*/
			/*if matched == 0 {
				return ctx, fmt.Errorf("no tasks matched label selector %v", source.Tasks.MatchLabels)
			}*/
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
