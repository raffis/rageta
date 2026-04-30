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

	"github.com/docker/cli/cli/config"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/client/llb/imagemetaresolver"
	gatewaypb "github.com/moby/buildkit/frontend/gateway/pb"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/auth/authprovider"
	"github.com/moby/buildkit/session/secrets/secretsprovider"
	"github.com/raffis/rageta/internal/utils/progressui"
	"github.com/tonistiigi/fsutil"
)

func WithRun(buildkit *client.Client, buildContext string, cacheImports []client.CacheOptionsEntry, cacheExports []client.CacheOptionsEntry, noCache bool) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if spec.Run == nil || buildkit == nil {
			return nil
		}
		return &Run{
			step:         *spec.Run,
			stepName:     spec.Name,
			buildkit:     buildkit,
			buildContext: buildContext,
			cacheImports: cacheImports,
			cacheExports: cacheExports,
			noCache:      noCache,
		}
	}
}

const defaultShell = "/bin/sh"

type Run struct {
	stepName     string
	step         v1beta1.RunStep
	buildkit     *client.Client
	buildContext string
	cacheImports []client.CacheOptionsEntry
	cacheExports []client.CacheOptionsEntry
	noCache      bool
}

func (s *Run) Bootstrap(_ Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		fmt.Printf("RUNNNNNN\n")
		run := s.step.DeepCopy()
		command, args := s.commandArgs(run)

		subst := []any{&run.Image, args, command, &run.WorkingDir, run.Guid, run.Uid}
		for i := range run.Caches {
			subst = append(subst, &run.Caches[i].ID, &run.Caches[i].Path)
		}
		for i := range run.Sources {
			switch {
			case run.Sources[i].Local != nil:
				subst = append(subst, &run.Sources[i].Local.Path, &run.Sources[i].Local.To)
			case run.Sources[i].Step != nil:
				subst = append(subst, &run.Sources[i].Step.Name, &run.Sources[i].Step.Path, &run.Sources[i].Step.To)
			default:
				return ctx, errors.New("no source type given")
			}
		}
		for i := range run.Artifacts {
			if run.Artifacts[i].Local != nil {
				subst = append(subst, &run.Artifacts[i].Local.Path, &run.Artifacts[i].Local.To)
			}
		}
		if err := substitute.Substitute(ctx.ToV1Beta1(), subst...); err != nil {
			return ctx, err
		}

		if run.Image == "" {
			return ctx, errors.New("image is required")
		}

		cmdline := append(append([]string(nil), command...), args...)
		if len(cmdline) == 0 {
			return ctx, errors.New("command, args, or script is required")
		}

		var runOpts []llb.RunOption

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
			runOpts = append(runOpts, llb.AddMount(c.Path, llb.Scratch(), llb.AsPersistentCacheDir(c.ID, sharing)))
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

		root := llb.Image(run.Image, imagemetaresolver.WithDefault)

		localMounts := make(map[string]fsutil.FS)

		for _, source := range run.Sources {
			switch {
			case source.Local != nil:
				if _, ok := localMounts["context"]; !ok {
					contextFS, err := fsutil.NewFS(s.buildContext)
					if err != nil {
						return ctx, fmt.Errorf("building context filesystem failed: %s", err)
					}
					localMounts["context"] = contextFS
				}

				srcPath := source.Local.Path
				if srcPath == "" {
					srcPath = "."
				}
				copyTo := source.Local.To
				if filepath.Clean(copyTo) == "." || copyTo == "" {
					if run.WorkingDir != "" {
						copyTo = run.WorkingDir
					} else {
						copyTo = "/"
					}
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
				root = root.File(
					llb.Copy(contextSrc, copyFrom, copyTo, &llb.CopyInfo{
						CreateDestPath:      true,
						CopyDirContentsOnly: true,
					}),
					llb.WithCustomNamef("copy CONTEXT:%s → %s", srcPath, copyTo),
				)

			case source.Step != nil:
				stepCtx, ok := ctx.Steps[source.Step.Name]
				if !ok || stepCtx.LLBState == nil {
					return ctx, fmt.Errorf("source step %q dependency not found", source.Step.Name)
				}

				srcPath := source.Step.Path
				if srcPath == "" {
					srcPath = "/"
				}
				to := source.Step.To
				if to == "" {
					to = srcPath
				}

				var dstInScratch string
				if to == "/" || strings.HasSuffix(to, "/") {
					dstInScratch = "/"
				} else {
					dstInScratch = "/" + filepath.Base(to)
				}

				copyInfo := &llb.CopyInfo{CreateDestPath: true}
				if srcPath == "/" || strings.HasSuffix(srcPath, "/") {
					copyInfo.CopyDirContentsOnly = true
				}

				root = root.File(
					llb.Copy(*stepCtx.LLBState, srcPath, dstInScratch, copyInfo),
					llb.WithCustomNamef("copy %s:%s → %s", source.Step.Name, srcPath, dstInScratch),
				)
			default:
				return ctx, errors.New("no source type given")
			}
		}

		type outputArtifact struct {
			absPath  string
			hostPath string
			hasGlob  bool
		}
		var outputs []outputArtifact

		for i, artifact := range run.Artifacts {
			if artifact.Local == nil {
				continue
			}
			absPath := artifact.Local.Path
			if filepath.Clean(absPath) == "." || absPath == "" {
				if run.WorkingDir == "" {
					return ctx, fmt.Errorf("run step %q: artifact[%d]: path %q requires workingDir to be set", s.stepName, i, artifact.Local.Path)
				}
				absPath = run.WorkingDir
			} else if !filepath.IsAbs(absPath) {
				if run.WorkingDir != "" {
					absPath = filepath.Join(run.WorkingDir, absPath)
				} else {
					absPath = "/" + absPath
				}
			}
			hostPath := artifact.Local.To
			if hostPath == "" {
				hostPath = artifact.Local.Path
			}
			outputs = append(outputs, outputArtifact{
				absPath:  absPath,
				hostPath: hostPath,
				hasGlob:  strings.ContainsAny(absPath, "*?["),
			})
		}

		runOpts = append(runOpts, llb.Args(cmdline))
		exec := root.Run(runOpts...)

		toMarshal := exec.Root()
		if len(outputs) > 0 {
			export := llb.Scratch()
			for i := 0; i < len(outputs); i++ {
				export = export.File(llb.Copy(exec.Root(), outputs[i].absPath, fmt.Sprintf("/%d/", i), &llb.CopyInfo{
					CreateDestPath: true,
					AllowWildcard:  outputs[i].hasGlob,
				}))
			}
			toMarshal = export
		}

		def, err := toMarshal.Marshal(ctx)
		if err != nil {
			return ctx, err
		}

		opt := client.SolveOpt{
			LocalMounts:  localMounts,
			CacheImports: s.cacheImports,
			CacheExports: s.cacheExports,
			Session: []session.Attachable{
				authprovider.NewDockerAuthProvider(authprovider.DockerAuthProviderConfig{
					AuthConfigProvider: authprovider.LoadAuthConfig(config.LoadDefaultConfigFile(ctx.Streams.Stderr)),
				}),
				secretsprovider.FromMap(secrets),
			},
		}

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

		if err := s.solve(ctx, def, opt); err != nil {
			var exitErr *gatewaypb.ExitError
			if errors.As(err, &exitErr) {
				return ctx, &ContainerError{
					containerName: s.stepName,
					image:         run.Image,
					exitCode:      int(exitErr.ExitCode),
					err:           err,
				}
			}
			return ctx, err
		}

		combined := exec.Root()
		ctx.LLBState = &combined

		for i, om := range outputs {
			src := filepath.Join(tmpDir, fmt.Sprintf("%d", i))
			if err := syncExportDirToHost(src, om.hostPath); err != nil {
				return ctx, fmt.Errorf("export %s: %w", om.absPath, err)
			}
		}

		return next(ctx)
	}, nil
}

func (s *Run) solve(ctx StepContext, def *llb.Definition, opt client.SolveOpt) error {
	ch := make(chan *client.SolveStatus)
	var solveErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, solveErr = s.buildkit.Solve(ctx, def, opt, ch)
	}()

	d, err := progressui.NewDisplay(ctx.Events.Dev, ctx.Streams.Stdout, progressui.PlainMode)
	if err != nil {
		return err
	}

	_, err = d.UpdateFrom(ctx, ch)
	if err != nil {
		return err
	}

	<-done
	return solveErr
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

func (s *Run) commandArgs(run *v1beta1.RunStep) (cmd []string, args []string) {
	script := strings.TrimSpace(run.Script)
	if strings.HasPrefix(script, "#!") {
		lines := strings.Split(script, "\n")
		shebang := strings.Split(lines[0], "#!")
		cmd = []string{shebang[1]}
		args = append(args, "-e", "-c", strings.Join(lines[1:], "\n"))
	} else {
		args = append([]string{defaultShell}, "-e", "-c", script)
	}
	return
}
