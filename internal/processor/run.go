package processor

import (
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/raffis/rageta/internal/runtime"
	"github.com/raffis/rageta/internal/substitute"
	"github.com/raffis/rageta/internal/utils"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"

	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/client/llb/imagemetaresolver"
	"github.com/moby/buildkit/util/progress/progressui"
)

func WithRun(buildkit *client.Client, outputFactory OutputFactory) ProcessorBuilder {
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

func (s *Run) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		run := s.step.DeepCopy()
		pod := &runtime.Pod{
			Name: fmt.Sprintf("rageta-%s-%s-%s", pipeline.ID(), ctx.UniqueID(), utils.RandString(5)),
			Spec: runtime.PodSpec{},
		}

		if err := substitute.Substitute(ctx.ToV1Beta1(), run.Guid, run.Uid); err != nil {
			return ctx, err
		}

		envs := make(map[string]string)
		maps.Copy(envs, ctx.EnvVars.Envs)
		maps.Copy(envs, ctx.SecretVars.Secrets)

		command, args := s.commandArgs(run)

		container := runtime.ContainerSpec{
			Name:    s.stepName,
			Stdin:   ctx.Streams.Stdin != nil || run.Stdin,
			TTY:     run.TTY,
			Image:   run.Image,
			Command: command,
			Args:    args,
			Env:     envs,
			PWD:     run.WorkingDir,
		}

		if run.Guid != nil {
			guid := run.Guid.IntValue()
			container.Guid = &guid
		}

		if run.Uid != nil {
			uid := run.Uid.IntValue()
			container.Uid = &uid
		}

		for _, vol := range run.VolumeMounts {
			container.Volumes = append(container.Volumes, runtime.Volume{
				Name:     vol.Name,
				HostPath: vol.HostPath,
				Path:     vol.MountPath,
				ReadOnly: vol.ReadOnly,
				Output:   vol.Output,
			})
		}

		if ctx.Template.Template != nil {
			if err := substitute.Substitute(ctx.ToV1Beta1(), ctx.Template.Template.Guid, ctx.Template.Template.Uid); err != nil {
				return ctx, err
			}

			ContainerSpec(&container, ctx.Template.Template)
			mergeTemplateCaches(run, ctx.Template.Template)
		}

		subst := []any{
			&container.Image,
			container.Args,
			container.Command,
			&container.PWD,
		}

		for i := range container.Volumes {
			subst = append(subst, &container.Volumes[i].HostPath, &container.Volumes[i].Path)
		}

		for i := range run.Caches {
			subst = append(subst, &run.Caches[i].ID, &run.Caches[i].Path)
		}

		if err := substitute.Substitute(ctx.ToV1Beta1(), subst...); err != nil {
			return ctx, err
		}

		for i := range container.Volumes {
			if container.Volumes[i].ReadOnly {
				container.Volumes[i].Output = false
			}
		}

		for i, vol := range container.Volumes {
			if vol.HostPath == "" {
				continue
			}
			srcPath, err := filepath.Abs(vol.HostPath)
			if err != nil {
				return ctx, fmt.Errorf("failed to get absolute path: %w", err)
			}

			container.Volumes[i].HostPath = srcPath
		}

		if run.Stdin && ctx.Streams.Stdin == nil {
			ctx.Streams.Stdin = os.Stdin
		}

		pod.Spec.Containers = []runtime.ContainerSpec{container}

		_, _ = ctx.Events.Dev.Write([]byte(fmt.Sprintf("🐋 starting %s", container.Image) + "\n"))
		fmt.Println(strings.Join(append(command, args...), " "))

		if err := s.solveWithBuildKit(ctx, container, run); err != nil {
			return ctx, err
		}

		/*ctx, err := s.exec(ctx, pod)

		if err != nil {
			var exitCode int
			var runtimeErr ExitCode
			if errors.As(err, &runtimeErr) {
				exitCode = runtimeErr.ExitCode()
			}

			return ctx, &ContainerError{
				containerName: pod.Name,
				image:         container.Image,
				exitCode:      exitCode,
				err:           err,
			}
		}*/

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

func ContainerSpec(container *runtime.ContainerSpec, template *v1beta1.Template) {
	if len(container.Args) == 0 {
		container.Args = template.Args
	}

	if len(container.Command) == 0 {
		container.Command = template.Command
	}

	if container.PWD == "" {
		container.PWD = template.WorkingDir
	}

	if container.Image == "" {
		container.Image = template.Image
	}

	if container.Uid == nil && template.Uid != nil {
		uid := template.Uid.IntValue()
		container.Uid = &uid
	}

	if container.Guid == nil && template.Guid != nil {
		guid := template.Guid.IntValue()
		container.Uid = &guid
	}

	for _, templateVol := range template.VolumeMounts {
		hasVolume := false
		for _, containerVol := range container.Volumes {
			if templateVol.Name == containerVol.Name {
				hasVolume = true
				break
			}
		}

		if !hasVolume {
			container.Volumes = append(container.Volumes, runtime.Volume{
				Name:     templateVol.Name,
				HostPath: templateVol.HostPath,
				Path:     templateVol.MountPath,
				ReadOnly: templateVol.ReadOnly,
				Output:   templateVol.Output,
			})
		}
	}
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

func (s *Run) solveWithBuildKit(ctx StepContext, container runtime.ContainerSpec, run *v1beta1.RunStep) error {
	if container.Image == "" {
		return fmt.Errorf("run step %q: image is required", s.stepName)
	}

	cmdline := append(append([]string(nil), container.Command...), container.Args...)
	if len(cmdline) == 0 {
		return fmt.Errorf("run step %q: command, args, or script is required", s.stepName)
	}

	localDirs := make(map[string]string)
	var runOpts []llb.RunOption

	for i, vol := range container.Volumes {
		if vol.HostPath == "" {
			if vol.Output {
				return fmt.Errorf("volume %q: output requires hostPath", vol.Name)
			}
			if vol.Path != "" {
				return fmt.Errorf("volume %q: hostPath is required for bind mounts", vol.Name)
			}
			continue
		}
		if vol.Output && vol.ReadOnly {
			return fmt.Errorf("volume %q: output and readOnly are mutually exclusive", vol.Name)
		}

		key := fmt.Sprintf("mount-%d", i)
		localDirs[key] = vol.HostPath
		src := llb.Local(key)

		var mo []llb.MountOption
		if vol.ReadOnly {
			mo = append(mo, llb.Readonly)
		}
		runOpts = append(runOpts, llb.AddMount(vol.Path, src, mo...))
	}

	for _, c := range run.Caches {
		if c.ID == "" || c.Path == "" {
			return fmt.Errorf("run step %q: cache mount requires id and path", s.stepName)
		}
		sharing := llb.CacheMountShared
		switch strings.ToLower(strings.TrimSpace(c.Sharing)) {
		case "", "shared":
		case "private":
			sharing = llb.CacheMountPrivate
		case "locked":
			sharing = llb.CacheMountLocked
		default:
			return fmt.Errorf("run step %q: cache sharing %q: want shared, private, or locked", s.stepName, c.Sharing)
		}
		runOpts = append(runOpts,
			llb.AddMount(c.Path, llb.Scratch(), llb.AsPersistentCacheDir(c.ID, sharing)),
		)
	}

	for k, v := range container.Env {
		runOpts = append(runOpts, llb.AddEnv(k, v))
	}
	if container.PWD != "" {
		runOpts = append(runOpts, llb.Dir(container.PWD))
	}
	runOpts = append(runOpts, llb.Args(cmdline))

	// Without image config (PATH, WORKDIR, ENV), e.g. golang images lack /usr/local/go/bin on PATH → exit 127.
	exec := llb.Image(container.Image, imagemetaresolver.WithDefault).Run(runOpts...)

	type outputMount struct {
		mountPath, hostPath string
	}
	var outputs []outputMount
	for _, vol := range container.Volumes {
		if vol.Output && vol.HostPath != "" && !vol.ReadOnly {
			outputs = append(outputs, outputMount{vol.Path, vol.HostPath})
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
		return err
	}

	ch := make(chan *client.SolveStatus)
	opt := client.SolveOpt{LocalDirs: localDirs}

	var tmpDir string
	if len(outputs) > 0 {
		tmpDir, err = os.MkdirTemp("", "rageta-buildkit-export-*")
		if err != nil {
			return err
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
		x, e := s.buildkit.Solve(ctx, def, opt, ch)
		solveErr = e

		fmt.Printf("x: %#v\n", x)
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
		return solveErr
	}

	for i, om := range outputs {
		src := filepath.Join(tmpDir, fmt.Sprintf("%d", i))
		if err := syncExportDirToHost(src, om.hostPath); err != nil {
			return fmt.Errorf("export %s: %w", om.mountPath, err)
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

/*
func (s *Run) exec(ctx StepContext, pod *runtime.Pod) (StepContext, error) {
	if len(pod.Spec.Containers[0].Command) > 0 || len(pod.Spec.Containers[0].Args) > 0 {
		cmd := strings.Join(append(pod.Spec.Containers[0].Command, pod.Spec.Containers[0].Args...), " ")
		w := xio.NewLineWriter(xio.NewPrefixWriter(ctx.Events.Dev, []byte("$ ")))
		w.Write([]byte(cmd))
		w.Flush()
	}

	await, err := s.driver.CreatePod(ctx, pod, ctx.Streams.Stdin,
		io.MultiWriter(append(ctx.Streams.AdditionalStdout, ctx.Streams.Stdout)...),
		io.MultiWriter(append(ctx.Streams.AdditionalStderr, ctx.Streams.Stderr)...),
	)

	if err != nil {
		return ctx, err
	}

	for _, v := range pod.Status.Containers {
		ctx.Containers[v.Name] = v
	}

	err = await.Wait(ctx)
	return ctx, err
}
*/
