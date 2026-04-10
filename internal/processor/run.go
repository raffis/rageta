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
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/secrets/secretsprovider"
	"github.com/moby/buildkit/util/progress/progressui"
)

func WithRun(buildkit *client.Client) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if spec.Run == nil || buildkit == nil {
			return nil
		}

		return &Run{
			step:     *spec.Run,
			stepName: spec.Name,
			buildkit: buildkit,
		}
	}
}

const (
	defaultShell = "/bin/sh"
)

type Run struct {
	stepName string
	step     v1beta1.RunStep
	buildkit *client.Client
}

/*

	if err := substitute.Substitute(ctx.ToV1Beta1(), run.Guid, run.Uid); err != nil {
		return ctx, err
	}

	envs := make(map[string]string)
	maps.Copy(envs, ctx.EnvVars.Envs)
	maps.Copy(envs, ctx.SecretVars.Secrets)

	command, args := s.commandArgs(run)

	spec := StepContainerFields{
		Image:   run.Image,
		Command: command,
		Args:    args,
		Env:     envs,
		PWD:     run.WorkingDir,
	}

	if run.Guid != nil {
		guid := run.Guid.IntValue()
		spec.Guid = &guid
	}

	if run.Uid != nil {
		uid := run.Uid.IntValue()
		spec.Uid = &uid
	}

	for _, vol := range run.VolumeMounts {
		spec.Volumes = append(spec.Volumes, StepVolumeMount{
			Name:      vol.Name,
			HostPath:  vol.HostPath,
			MountPath: vol.MountPath,
			ReadOnly:  vol.ReadOnly,
			Output:    vol.Output,
		})
	}

	if ctx.Template.Template != nil {
		if err := substitute.Substitute(ctx.ToV1Beta1(), ctx.Template.Template.Guid, ctx.Template.Template.Uid); err != nil {
			return ctx, err
		}

		ApplyTemplateToStepFields(&spec, ctx.Template.Template)
		mergeTemplateCaches(run, ctx.Template.Template)
	}

	subst := []any{
		&spec.Image,
		spec.Args,
		spec.Command,
		&spec.PWD,
	}

	for i := range spec.Volumes {
		subst = append(subst, &spec.Volumes[i].HostPath, &spec.Volumes[i].MountPath)
	}

	for i := range run.Caches {
		subst = append(subst, &run.Caches[i].ID, &run.Caches[i].Path)
	}

	if err := substitute.Substitute(ctx.ToV1Beta1(), subst...); err != nil {
		return ctx, err
	}

	for i := range spec.Volumes {
		if spec.Volumes[i].ReadOnly {
			spec.Volumes[i].Output = false
		}
	}

	for i, vol := range spec.Volumes {
		if vol.HostPath == "" {
			continue
		}
		srcPath, err := filepath.Abs(vol.HostPath)
		if err != nil {
			return ctx, fmt.Errorf("failed to get absolute path: %w", err)
		}

		spec.Volumes[i].HostPath = srcPath
	}

	if run.Stdin && ctx.Streams.Stdin == nil {
		ctx.Streams.Stdin = os.Stdin
	}

	_, _ = ctx.Events.Dev.Write([]byte(fmt.Sprintf("🐋 starting %s", spec.Image) + "\n"))
	fmt.Println(strings.Join(append(command, args...), " "))


*/

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

		/*for i := range run.VolumeMounts {
			subst = append(subst, &spec.Volumes[i].HostPath, &spec.Volumes[i].MountPath)
		}*/

		for i := range run.Caches {
			subst = append(subst, &run.Caches[i].ID, &run.Caches[i].Path)
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

		runOpts = append(runOpts, llb.Args(cmdline))
		localDirs := make(map[string]string)

		for i, vol := range run.VolumeMounts {
			if vol.HostPath == "" {
				if vol.Output {
					return ctx, fmt.Errorf("volume %q: output requires hostPath", vol.Name)
				}
				if vol.MountPath != "" {
					return ctx, fmt.Errorf("volume %q: hostPath is required for bind mounts", vol.Name)
				}
				continue
			}
			if vol.Output && vol.ReadOnly {
				return ctx, fmt.Errorf("volume %q: output and readOnly are mutually exclusive", vol.Name)
			}

			key := fmt.Sprintf("mount-%d", i)
			localDirs[key] = vol.HostPath
			src := llb.Local(key)

			var mo []llb.MountOption
			if vol.ReadOnly {
				mo = append(mo, llb.Readonly)
			}
			runOpts = append(runOpts, llb.AddMount(vol.MountPath, src, mo...))
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
				return ctx, fmt.Errorf("run step %q: cache sharing %q: want shared, private, or locked", s.stepName, c.Sharing)
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

		exec := llb.Image(run.Image, imagemetaresolver.WithDefault).Run(runOpts...)

		type outputMount struct {
			mountPath, hostPath string
		}
		var outputs []outputMount
		for _, vol := range run.VolumeMounts {
			if vol.Output && vol.HostPath != "" && !vol.ReadOnly {
				outputs = append(outputs, outputMount{vol.MountPath, vol.HostPath})
			}
		}

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
			LocalDirs: localDirs,
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

func mergeTemplateCaches(run *v1beta1.RunStep, tmpl *v1beta1.Template) {
	for _, c := range tmpl.Caches {
		dup := false
		for _, ex := range run.Caches {
			if ex.ID == c.ID && ex.Path == c.Path {
				dup = true
				break
			}
		}
		if !dup {
			run.Caches = append(run.Caches, c)
		}
	}
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
	args = run.Args

	if script == "" {
		return run.Command, run.Args
	}

	hasShebang := strings.HasPrefix(script, "#!")
	if hasShebang {
		lines := strings.Split(script, "\n")
		header := lines[0]
		shebang := strings.Split(header, "#!")
		cmd = []string{shebang[1]}
		args = append(args, "-e", "-c", strings.Join(lines[1:], "\n"))
	} else {
		if len(run.Command) == 0 {
			cmd = []string{defaultShell}
		} else {
			cmd = run.Command
		}

		args = append(args, "-e", "-c", script)
	}

	return
}
