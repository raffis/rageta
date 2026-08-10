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
		if spec.Artifacts == nil || spec.Service != nil {
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
			case artifacts[i].EnvVars != nil:
				subst = append(subst, &artifacts[i].EnvVars.Path)
			case artifacts[i].OutputVars != nil:
				subst = append(subst, &artifacts[i].OutputVars.Path)
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
			case artifact.EnvVars != nil:
				srcPath = artifact.EnvVars.Path
			case artifact.OutputVars != nil:
				srcPath = artifact.OutputVars.Path
			case artifact.Local != nil:
				srcPath = artifact.Local.Path
				if srcPath == "" {
					srcPath = "."
				}

				hostPath = artifact.Local.To
				if hostPath == "" {
					hostPath = "."
				}
			case artifact.Image != nil:
				continue
			default:
				return ctx, errors.New("unknown artifact type")
			}

			exportDef, err := llb.Scratch().File(llb.Copy(ctx.Build.State, srcPath, "/", &llb.CopyInfo{
				CreateDestPath:      true,
				CopyDirContentsOnly: true,
				AllowWildcard:       strings.ContainsAny(srcPath, "*?["),
			})).Marshal(ctx)

			if err != nil {
				return ctx, fmt.Errorf("artifact %q: marshal: %w", srcPath, err)
			}

			exportRef, err := s.solve(ctx, exportDef)
			if err != nil {
				return ctx, fmt.Errorf("artifact %q: solve: %w", srcPath, err)
			}

			switch {
			case artifact.EnvVars != nil:
				vars, err := readVars(ctx, exportRef, srcPath)
				if err != nil {
					return ctx, fmt.Errorf("envvar artifact failed %q: %w", srcPath, err)
				}

				maps.Copy(ctx.EnvVars.Envs, vars)
			case artifact.OutputVars != nil:
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

		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := copyFileToLocal(ctx, ref, src, dst, st); err != nil {
			return err
		}
	}

	return nil
}

const readFileChunkSize = 8 << 20

func copyFileToLocal(ctx context.Context, ref gwclient.Reference, src, dst string, st *fstypes.Stat) error {
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fs.FileMode(st.Mode))
	if err != nil {
		return err
	}
	defer f.Close()

	if st.Size == 0 {
		return nil
	}

	for offset := int64(0); offset < st.Size; offset += readFileChunkSize {
		length := min(readFileChunkSize, st.Size-offset)

		data, err := ref.ReadFile(ctx, gwclient.ReadRequest{
			Filename: src,
			Range:    &gwclient.FileRange{Offset: int(offset), Length: int(length)},
		})
		if err != nil {
			return err
		}

		if _, err := f.Write(data); err != nil {
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
