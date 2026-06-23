package processor

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	"github.com/raffis/rageta/internal/substitute"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"

	"github.com/moby/buildkit/client/llb"
	gwclient "github.com/moby/buildkit/frontend/gateway/client"
	fstypes "github.com/tonistiigi/fsutil/types"
)

func WithArtifacts(gwClient gwclient.Client) ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		if spec.Artifacts == nil {
			return nil
		}

		return &Artifacts{
			gwClient:  gwClient,
			artifacts: spec.Artifacts,
		}
	}
}

type Artifacts struct {
	gwClient  gwclient.Client
	artifacts []v1beta1.Artifact
}

func (s *Artifacts) Bootstrap(_ Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		artifacts := make([]v1beta1.Artifact, len(s.artifacts))
		subst := []any{}

		for i := range artifacts {
			artifacts[i] = *s.artifacts[i].DeepCopy()
			switch {
			case artifacts[i].Envvars != nil:
				subst = append(subst, &artifacts[i].Envvars.Path)
			case artifacts[i].Outputvars != nil:
				subst = append(subst, &artifacts[i].Outputvars.Path)
			case artifacts[i].Local != nil:
				subst = append(subst, &artifacts[i].Local.Path, &artifacts[i].Local.To)
			case artifacts[i].Image != nil:
			default:
				return ctx, errors.New("unknown artifact type")
			}
		}

		if err := substitute.Substitute(ctx.ToV1Beta1(), subst...); err != nil {
			return ctx, err
		}

		ctx, err := next(ctx)
		if err != nil {
			return ctx, err
		}

		for _, artifact := range artifacts {
			var srcPath, hostPath string

			switch {
			case artifact.Envvars != nil:
				srcPath = artifact.Envvars.Path
			case artifact.Outputvars != nil:
				srcPath = artifact.Outputvars.Path
			case artifact.Local != nil:
				srcPath := artifact.Local.Path
				if srcPath == "" {
					srcPath = "."
				}

				hostPath := artifact.Local.To
				if hostPath == "" {
					hostPath = srcPath
				}
			case artifact.Image != nil:
				continue
			default:
				return ctx, errors.New("unknown artifact type")
			}

			exportDef, err := llb.Scratch().File(llb.Copy(ctx.Build.State, srcPath, "/", &llb.CopyInfo{
				CreateDestPath: true,
				AllowWildcard:  strings.ContainsAny(srcPath, "*?["),
			})).Marshal(ctx)

			if err != nil {
				return ctx, fmt.Errorf("artifact %q: marshal: %w", srcPath, err)
			}

			exportRef, err := s.solve(ctx, exportDef)
			if err != nil {
				return ctx, fmt.Errorf("artifact %q: solve: %w", srcPath, err)
			}

			switch {
			case artifact.Envvars != nil:
				vars, err := readVars(ctx, exportRef, srcPath)
				if err != nil {
					return ctx, fmt.Errorf("envvar artifact failed %q: %w", srcPath, err)
				}

				maps.Copy(ctx.EnvVars.Envs, vars)
			case artifact.Outputvars != nil:
			case artifact.Local != nil:
				if err := readObject(ctx, exportRef, "/", hostPath); err != nil {
					return ctx, fmt.Errorf("local artifact failed %q: %w", hostPath, err)
				}
			case artifact.Image != nil:
				continue
			default:
				return ctx, errors.New("unknown artifact type")
			}
		}

		return ctx, nil
	}, nil
}

func (s *Artifacts) solve(ctx context.Context, def *llb.Definition) (gwclient.Reference, error) {
	res, err := s.gwClient.Solve(ctx, gwclient.SolveRequest{
		Definition: def.ToPB(),
		Evaluate:   true,
	})

	if err != nil {
		return nil, err
	}

	return res.Ref, nil
}

func readObject(ctx context.Context, ref gwclient.Reference, srcPath, hostPath string) error {
	stat, err := ref.StatFile(ctx, gwclient.StatRequest{Path: srcPath})
	if err != nil {
		return err
	}

	if !stat.IsDir() {
		return exportObject(ctx, ref, srcPath, hostPath, []*fstypes.Stat{stat})
	}

	stats, err := ref.ReadDir(ctx, gwclient.ReadDirRequest{
		Path:           srcPath,
		IncludePattern: "**",
	})

	if err != nil {
		return err
	}

	return exportObject(ctx, ref, srcPath, hostPath, stats)
}

func exportObject(ctx context.Context, ref gwclient.Reference, base, hostBase string, stats []*fstypes.Stat) error {
	for _, st := range stats {
		rel := strings.TrimPrefix(st.Path, "/")
		src := filepath.Join(base, rel)
		dst := filepath.Join(hostBase, rel)

		if st.IsDir() {
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return err
			}
			sub, err := ref.ReadDir(ctx, gwclient.ReadDirRequest{Path: src, IncludePattern: "**"})
			if err != nil {
				return err
			}

			return exportObject(ctx, ref, src, dst, sub)
		}

		data, err := ref.ReadFile(ctx, gwclient.ReadRequest{Filename: src})
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, data, fs.FileMode(st.Mode)); err != nil {
			return err
		}
	}

	return nil
}

func readVars(ctx context.Context, ref gwclient.Reference, srcPath string) (map[string]string, error) {
	stat, err := ref.StatFile(ctx, gwclient.StatRequest{Path: srcPath})
	if err != nil {
		return nil, err
	}

	if stat.IsDir() {
		return nil, errors.New("must be a file")
	}

	b, err := ref.ReadFile(ctx, gwclient.ReadRequest{Filename: srcPath})
	if err != nil {
		return nil, err
	}

	vars, err := godotenv.UnmarshalBytes(b)
	if err != nil {
		return vars, fmt.Errorf("vars parser failed: %w", err)
	}

	return vars, err
}

/*
	for name, output := range outputs {
		_ = output.Sync()
		b, err := io.ReadAll(output)
		if err != nil {
			return ctx, err
		}

		value := v1beta1.ParamValue{}

		if err := value.UnmarshalJSON(b); err != nil {
			return ctx, fmt.Errorf("param output failed: %w", err)
		}

		ctx.OutputVars.OutputVars[name] = value
	}

*/
