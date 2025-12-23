package processor

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/raffis/rageta/internal/runtime"
	"github.com/raffis/rageta/internal/substitute"
	"github.com/raffis/rageta/internal/utils"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithRun(defaultPullPolicy runtime.PullImagePolicy, driver runtime.Interface, outputFactory OutputFactory, teardown chan Teardown, handlerBinaryPath string) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if spec.Run == nil {
			return nil
		}

		return &Run{
			step:              *spec.Run,
			stepName:          spec.Name,
			driver:            driver,
			defaultPullPolicy: defaultPullPolicy,
			teardown:          teardown,
			handlerBinaryPath: handlerBinaryPath,
		}
	}
}

const (
	defaultScriptHeader = "#!/bin/sh\nset -euo pipefail\n"
)

type Run struct {
	stepName          string
	step              v1beta1.RunStep
	driver            runtime.Interface
	defaultPullPolicy runtime.PullImagePolicy
	teardown          chan Teardown
	handlerBinaryPath string
}

func (s *Run) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		run := s.step.DeepCopy()
		pod := &runtime.Pod{
			Name: fmt.Sprintf("%s-%s-%s", SuffixName(s.stepName, ctx.NamePrefix), pipeline.ID(), utils.RandString(5)),
			Spec: runtime.PodSpec{},
		}

		if err := substitute.Substitute(ctx.ToV1Beta1(), run.Guid, run.Uid); err != nil {
			return ctx, err
		}

		envs := make(map[string]string)
		maps.Copy(envs, ctx.Envs)
		maps.Copy(envs, ctx.Secrets)

		command, err := s.commandArgs(run, ctx)
		if err != nil {
			return ctx, err
		}

		container := runtime.ContainerSpec{
			Name:            s.stepName,
			Stdin:           ctx.Stdin != nil || run.Stdin,
			TTY:             run.TTY,
			Image:           run.Image,
			ImagePullPolicy: s.defaultPullPolicy,
			Command:         command,
			Env:             envs,
			PWD:             run.WorkingDir,
			RestartPolicy:   runtime.RestartPolicy(run.RestartPolicy),
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
			})
		}

		if ctx.Template != nil {
			if err := substitute.Substitute(ctx.ToV1Beta1(), ctx.Template.Guid, ctx.Template.Uid); err != nil {
				return ctx, err
			}

			ContainerSpec(&container, ctx.Template)
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

		if err := substitute.Substitute(ctx.ToV1Beta1(), subst...); err != nil {
			return ctx, err
		}

		for i, vol := range container.Volumes {
			srcPath, err := filepath.Abs(vol.HostPath)
			if err != nil {
				return ctx, fmt.Errorf("failed to get absolute path: %w", err)
			}

			container.Volumes[i].HostPath = srcPath
		}

		if run.Stdin && ctx.Stdin == nil {
			ctx.Stdin = os.Stdin
		}

		pod.Spec.Containers = []runtime.ContainerSpec{container}
		ctx, err = s.exec(ctx, pod)

		if err != nil {
			return ctx, fmt.Errorf("container %s failed: %w", pod.Name, err)
		}

		//TODO: is this at the right place?
		ctx.StdinPath = ""

		return next(ctx)
	}, nil
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
			})
		}
	}
}

func (s *Run) commandArgs(run *v1beta1.RunStep, ctx StepContext) ([]string, error) {
	script := strings.TrimSpace(run.Script)
	args := run.Args
	useHandler := len(ctx.AdditionalStdoutPaths) > 0 || len(ctx.AdditionalStderrPaths) > 0 || ctx.StdinPath != ""
	var cmd []string
	entrypoint := run.Command

	if len(entrypoint) == 0 && script == "" {
		ref, err := name.ParseReference(run.Image)
		if err != nil {
			return cmd, err
		}

		img, err := remote.Image(ref)
		if err != nil {
			return cmd, err
		}

		cfg, err := img.ConfigFile()
		if err != nil {
			return cmd, err
		}

		entrypoint = cfg.Config.Entrypoint
	}

	if useHandler {
		cmd = []string{s.handlerBinaryPath}

		if ctx.StdinPath != "" {
			cmd = append(cmd, "--stdin", ctx.StdinPath)
		}

		for _, path := range ctx.AdditionalStdoutPaths {
			cmd = append(cmd, "--stdout", path)
		}

		for _, path := range ctx.AdditionalStderrPaths {
			cmd = append(cmd, "--stderr", path)
		}

		cmd = append(cmd, "--")
	}

	if script == "" {
		cmd = append(cmd, entrypoint...)
		cmd = append(cmd, args...)

		return cmd, nil
	}

	hasShebang := strings.HasPrefix(script, "#!")

	if !hasShebang {
		script = defaultScriptHeader + script
	}

	header := strings.Split(script, "\n")[0]
	shebang := strings.Split(header, "#!")

	cmd = append(cmd, shebang[1])
	cmd = append(cmd, "-c", script)

	return cmd, nil
}

func (s *Run) exec(ctx StepContext, pod *runtime.Pod) (StepContext, error) {
	await, err := s.driver.CreatePod(ctx, pod, ctx.Stdin, ctx.Stdout, ctx.Stderr)

	if err != nil {
		return ctx, err
	}

	for _, v := range pod.Status.Containers {
		ctx.Containers[v.Name] = v
	}

	if s.step.Await == v1beta1.AwaitStatusReady {
		done := make(chan error)
		go func() {
			if err := await.Wait(); err != nil {
				done <- err
			}

			done <- nil
		}()

		s.teardown <- func(teardownCtx context.Context, timeout time.Duration) error {
			//In case of Await == v1beta1.AwaitStatusReady we need to delete the container here
			//otherwise we end up with orphaned running containers
			//And in addition if the container here is not deleted the done channel will never be fulfilled as there is nothing which will stop
			//the containers otherwise if the app was started with --no-gc (skip pod deletion)
			if containerStatus, ok := ctx.Containers[s.stepName]; ok {
				err := s.driver.DeletePod(teardownCtx, &runtime.Pod{
					Status: runtime.PodStatus{
						Containers: []runtime.ContainerStatus{containerStatus},
					},
				}, timeout)

				if err != nil {
					return err
				}
			}

			return <-done
		}
	} else {
		if err := await.Wait(); err != nil {
			return ctx, err
		}
	}

	return ctx, err
}
