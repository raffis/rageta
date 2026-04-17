package processor

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/raffis/rageta/internal/substitute"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"

	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/client/llb/imagemetaresolver"
	gatewaypb "github.com/moby/buildkit/frontend/gateway/pb"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/secrets/secretsprovider"
	"github.com/moby/buildkit/util/progress/progressui"
	"github.com/tonistiigi/fsutil"
)

func WithRun(buildkit *client.Client, cacheImports []client.CacheOptionsEntry, cacheExports []client.CacheOptionsEntry, noCache bool) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if spec.Run == nil || buildkit == nil {
			return nil
		}

		return &Run{
			step:         *spec.Run,
			stepName:     spec.Name,
			buildkit:     buildkit,
			cacheImports: cacheImports,
			cacheExports: cacheExports,
			noCache:      noCache,
		}
	}
}

const (
	defaultShell = "/bin/sh"
)

type Run struct {
	stepName     string
	step         v1beta1.RunStep
	buildkit     *client.Client
	cacheImports []client.CacheOptionsEntry
	cacheExports []client.CacheOptionsEntry
	noCache      bool
}

func (s *Run) Bootstrap(_ Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		run := s.step.DeepCopy()
		command, args := s.commandArgs(run)

		subst := []any{
			&run.Image,
			args,
			command,
			&run.WorkingDir,
			run.Guid,
			run.Uid,
		}

		for _, cache := range run.Caches {
			subst = append(subst, &cache.ID, &cache.Path)
		}

		for _, source := range run.Sources {
			switch {
			case source.Local != nil:
				subst = append(subst, &source.Local.Path, &source.Local.To)
			default:
				return ctx, errors.New("no source type given")
			}
		}

		for _, artifact := range run.Artifacts {
			switch {
			case artifact.Local != nil:
				subst = append(subst, &artifact.Local.Path, &artifact.Local.To)
			default:
				return ctx, errors.New("no artifact type given")
			}
		}

		if err := substitute.Substitute(ctx.ToV1Beta1(), subst...); err != nil {
			return ctx, err
		}

		if run.Image == "" {
			return ctx, errors.New("image is required")
		}

		var runOpts []llb.RunOption

		cmdline := append(append([]string(nil), command...), args...)
		if len(cmdline) == 0 {
			return ctx, errors.New("command, args, or script is required")
		}

		for _, c := range run.Caches {
			if c.ID == "" || c.Path == "" {
				return ctx, errors.New("cache mount requires id and path")
			}
			sharing := llb.CacheMountShared
			switch strings.ToLower(strings.TrimSpace(c.Sharing)) {
			case "", "shared":
			case "private":
				sharing = llb.CacheMountPrivate
			case "locked":
				sharing = llb.CacheMountLocked
			default:
				return ctx, fmt.Errorf("cache sharing %q: want shared, private, or locked", c.Sharing)
			}
			runOpts = append(runOpts,
				llb.AddMount(c.Path, llb.Scratch(), llb.AsPersistentCacheDir(c.ID, sharing)),
			)
		}

		for k, v := range ctx.EnvVars.Envs {
			runOpts = append(runOpts, llb.AddEnv(k, v))
		}

		secrets := make(map[string][]byte, len(ctx.SecretVars.Secrets))
		for k, v := range ctx.SecretVars.Secrets {
			runOpts = append(runOpts, llb.AddSecret(fmt.Sprintf("/run/secrets/%s", k), llb.SecretID(k)))
			secrets[k] = []byte(v)
		}

		if run.WorkingDir != "" {
			runOpts = append(runOpts, llb.Dir(run.WorkingDir))
		}

		if s.noCache {
			runOpts = append(runOpts, llb.IgnoreCache)
		}

		localMounts := make(map[string]fsutil.FS)
		for i, source := range run.Sources {
			switch {
			case source.Local != nil:
				if source.Local.Path == "" {
					source.Local.Path = "."
				}
				if source.Local.To == "" {
					source.Local.To = source.Local.Path
				}
				localName := fmt.Sprintf("local-src-%d", i)
				fs, err := fsutil.NewFS(source.Local.Path)
				if err != nil {
					return ctx, fmt.Errorf("run step %q: source[%d]: %w", s.stepName, i, err)
				}
				localMounts[localName] = fs
				runOpts = append(runOpts, llb.AddMount(source.Local.To, llb.Local(localName)))
			default:
				return ctx, errors.New("no source type given")
			}
		}

		type outputMount struct {
			mountPath, hostPath string
		}
		var outputs []outputMount

		for _, artifact := range run.Artifacts {
			switch {
			case artifact.Local != nil:
				if artifact.Local.Path == "" {
					artifact.Local.Path = "."
				}

				hostPath := artifact.Local.To
				if hostPath == "" {
					hostPath = artifact.Local.Path
				}
				outputs = append(outputs, outputMount{
					mountPath: artifact.Local.Path,
					hostPath:  hostPath,
				})
				runOpts = append(runOpts, llb.AddMount(artifact.Local.Path, llb.Scratch()))
			}
		}

		runOpts = append(runOpts, llb.Args([]string{"sh", "-c", "pwd; ls -hals"}))
		//runOpts = append(runOpts, llb.Args(cmdline))
		exec := llb.Image(run.Image, imagemetaresolver.WithDefault).Run(runOpts...)

		var export llb.State
		if len(outputs) == 0 {
			export = exec.Root()
		} else {
			action := llb.Copy(exec.GetMount(outputs[0].mountPath), "/", fmt.Sprintf("/%d/", 0))
			for i := 1; i < len(outputs); i++ {
				action = action.Copy(exec.GetMount(outputs[i].mountPath), "/", fmt.Sprintf("/%d/", i))
			}
			export = llb.Scratch().File(action)
		}

		def, err := export.Marshal(ctx)
		if err != nil {
			return ctx, err
		}

		ch := make(chan *client.SolveStatus)
		opt := client.SolveOpt{
			LocalMounts:  localMounts,
			CacheImports: s.cacheImports,
			CacheExports: s.cacheExports,
			Session: []session.Attachable{
				secretsprovider.FromMap(secrets),
			}}

		var tmpDir string
		if len(outputs) > 0 {
			tmpDir, err = os.MkdirTemp("", "rageta-buildkit-export-*")
			if err != nil {
				return ctx, err
			}
			defer os.RemoveAll(tmpDir)
			opt.Exports = []client.ExportEntry{{
				Type:      client.ExporterLocal,
				OutputDir: tmpDir,
			}}
		}

		var solveErr error
		done := make(chan struct{})
		go func() {
			defer close(done)
			_, solveErr = s.buildkit.Solve(ctx, def, opt, ch)
		}()

		d, err := progressui.NewDisplay(ctx.Streams.Stderr, progressui.TtyMode)
		if err != nil {
			// If an error occurs while attempting to create the tty display,
			// fallback to using plain mode on stdout (in contrast to stderr).
			d, _ = progressui.NewDisplay(ctx.Streams.Stderr, progressui.PlainMode)
		}
		// not using shared context to not disrupt display but let is finish reporting errors
		_, err = d.UpdateFrom(ctx, ch)

		//for status := range ch {

		/*for _, msg := range status.Logs {
			if msg.Stream == 1 {
				_, _ = ctx.Streams.Stdout.Write(msg.Data)
			} else {
				_, _ = ctx.Streams.Stderr.Write(msg.Data)
			}
		}*/
		//}

		<-done
		if solveErr != nil {

			var exitErr *gatewaypb.ExitError
			if errors.As(solveErr, &exitErr) {
				return ctx, &ContainerError{
					containerName: s.stepName,
					image:         run.Image,
					exitCode:      int(exitErr.ExitCode),
					err:           solveErr,
				}
			}

			fmt.Printf("solveErr: %#v", solveErr)
			return ctx, solveErr
		}

		for i, om := range outputs {
			src := filepath.Join(tmpDir, fmt.Sprintf("%d", i))
			if err := syncExportDirToHost(src, om.hostPath); err != nil {
				return ctx, fmt.Errorf("export %s: %w", om.mountPath, err)
			}
		}

		return next(ctx)
	}, nil
}

type ContainerError struct {
	containerName string
	image         string
	exitCode      int
	err           error
}

func (e *ContainerError) Error() string {
	return fmt.Sprintf("container failed: %s", e.err.Error())
}

func (e *ContainerError) Unwrap() error {
	return e.err
}

func (e *ContainerError) ContainerName() string {
	return e.containerName
}

func (e *ContainerError) ExitCode() int {
	return e.exitCode
}

func (e *ContainerError) Image() string {
	return e.image
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

func (s *Run) commandArgs(run *v1beta1.RunStep) (cmd []string, args []string) {
	script := strings.TrimSpace(run.Script)

	hasShebang := strings.HasPrefix(script, "#!")
	if hasShebang {
		lines := strings.Split(script, "\n")
		header := lines[0]
		shebang := strings.Split(header, "#!")
		cmd = []string{shebang[1]}
		args = append(args, "-e", "-c", strings.Join(lines[1:], "\n"))
	} else {
		args = append([]string{defaultShell}, "-e", "-c", script)
	}

	return
}
