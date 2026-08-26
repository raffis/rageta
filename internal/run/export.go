package run

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/raffis/rageta/internal/setup/flagset"

	gwclient "github.com/moby/buildkit/frontend/gateway/client"
	fstypes "github.com/tonistiigi/fsutil/types"
)

type ExportOptions struct {
	To string
}

func NewExportOptions() ExportOptions {
	return ExportOptions{}
}

func (s *ExportOptions) BindFlags(flags flagset.Interface) {
	flags.StringVarP(&s.To, "export", "", s.To, "Export every task's resulting filesystem to the given host directory, one subdirectory per task name")
}

func (s ExportOptions) Build() Task {
	return &Export{opts: s}
}

type Export struct {
	opts ExportOptions
}

func (s *Export) Label() string {
	return "Exporting task artifacts"
}

func (s *Export) Run(rc *RunContext, next Next) error {
	if s.opts.To == "" {
		return next(rc)
	}

	for name, taskCtx := range rc.Execute.ResultContext.Tasks {
		def, err := taskCtx.Build.State.Marshal(rc)
		if err != nil {
			return fmt.Errorf("export task %q: marshal: %w", name, err)
		}

		res, err := rc.Buildkit.GatewayClient.Solve(rc, gwclient.SolveRequest{
			Definition: def.ToPB(),
			Evaluate:   true,
		})
		if err != nil {
			return fmt.Errorf("export task %q: solve: %w", name, err)
		}

		hostPath := filepath.Join(s.opts.To, name)
		if err := os.MkdirAll(hostPath, 0o755); err != nil {
			return fmt.Errorf("export task %q: %w", name, err)
		}

		if err := readObject(rc, res.Ref, "/", hostPath); err != nil {
			return fmt.Errorf("export task %q: %w", name, err)
		}
	}

	return next(rc)
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
