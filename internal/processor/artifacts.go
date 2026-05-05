package processor

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/raffis/rageta/internal/substitute"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"

	"github.com/moby/buildkit/client/llb"
	gwclient "github.com/moby/buildkit/frontend/gateway/client"
	fstypes "github.com/tonistiigi/fsutil/types"
)

func WithArtifacts(gwClient gwclient.Client) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
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
	return func(ctx StepContext) (StepContext, error) {
		subst := []any{}
		for i := range s.artifacts {
			if s.artifacts[i].Local != nil {
				subst = append(subst, &s.artifacts[i].Local.Path, &s.artifacts[i].Local.To)
			}
		}
		if err := substitute.Substitute(ctx.ToV1Beta1(), subst...); err != nil {
			return ctx, err
		}

		ctx, err := next(ctx)
		if err != nil {
			return ctx, err
		}

		for _, artifact := range s.artifacts {
			if artifact.Local == nil {
				continue
			}
			srcPath := artifact.Local.Path
			if srcPath == "" {
				srcPath = "."
			}
			hostPath := artifact.Local.To
			if hostPath == "" {
				hostPath = srcPath
			}

			exportDef, err := llb.Scratch().File(llb.Copy(ctx.Build.State, srcPath, "/", &llb.CopyInfo{
				CreateDestPath: true,
				AllowWildcard:  strings.ContainsAny(srcPath, "*?["),
			})).Marshal(ctx)

			if err != nil {
				return ctx, fmt.Errorf("export %q: marshal: %w", srcPath, err)
			}

			exportRef, err := s.solve(ctx, exportDef)
			if err != nil {
				return ctx, fmt.Errorf("export %q: solve: %w", srcPath, err)
			}

			if err := exportRefToHost(ctx, exportRef, "/", hostPath); err != nil {
				return ctx, fmt.Errorf("export %q: %w", hostPath, err)
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

func exportRefToHost(ctx context.Context, ref gwclient.Reference, srcPath, hostPath string) error {
	stats, err := ref.ReadDir(ctx, gwclient.ReadDirRequest{
		Path:           srcPath,
		IncludePattern: "**",
	})
	if err != nil {
		return err
	}
	return writeStats(ctx, ref, srcPath, hostPath, stats)
}

func writeStats(ctx context.Context, ref gwclient.Reference, base, hostBase string, stats []*fstypes.Stat) error {
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
			if err := writeStats(ctx, ref, src, dst, sub); err != nil {
				return err
			}
		} else {
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
	}
	return nil
}

func syncExportDirToHost(exported, host string) error {
	if err := os.RemoveAll(host); err != nil {
		return fmt.Errorf("clear host path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(host), 0o755); err != nil {
		return err
	}
	if err := os.Rename(exported, host); err != nil {
		if err := copyDir(exported, host); err != nil {
			return err
		}
		_ = os.RemoveAll(exported)
	}
	return nil
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyRegularFile(path, target)
	})
}

func copyRegularFile(src, dst string) error {
	st, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() {
		return fmt.Errorf("copy %s: not a regular file", src)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, st.Mode().Perm())
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

/*

		envTmp, err := os.CreateTemp(path.Join(ctx.ContextDir, ctx.UniqueID()), "env")
		if err != nil {
			return ctx, err
		}

		var nextErr error
		defer func() {
			_ = envTmp.Close()
			_ = os.Remove(envTmp.Name())
		}()

		ctx.EnvVars.OutputPath = envTmp.Name()
		ctx, nextErr = next(ctx)
		if syncErr := envTmp.Sync(); syncErr != nil {
			nextErr = syncErr
		}

		envs, err := parseVars(envTmp)
		if err != nil {
			return ctx, err
		}

		maps.Copy(originEnvs, envs)
		ctx.EnvVars.Envs = originEnvs
		ctx.EnvVars.OutputPath = ""

		return ctx, nextErr

	}, nil
}

func envMap(envs []v1beta1.EnvVar, osEnv, defaultEnv map[string]string) map[string]string {
	env := make(map[string]string)
	for _, e := range envs {
		if e.Value == nil {
			if v, ok := osEnv[e.Name]; ok {
				env[e.Name] = v
			}

			continue
		}

		env[e.Name] = *e.Value
	}

	maps.Copy(env, defaultEnv)
	return env
}

*/
