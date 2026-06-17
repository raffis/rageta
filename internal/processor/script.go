package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/moby/buildkit/client/llb"
	gwclient "github.com/moby/buildkit/frontend/gateway/client"
	"github.com/raffis/rageta/internal/secrets"
	"github.com/raffis/rageta/internal/substitute"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithScript(store secrets.Interface) ProcessorBuilder {
	return func(spec *v1beta1.Step) Bootstraper {
		if spec.Script == nil {
			return nil
		}
		return &Script{
			script:   *spec.Script,
			stepName: spec.Name,
			store:    store,
		}
	}
}

const (
	defaultShell = "/bin/ash"
	scriptPath   = "/rageta/script.sh"
	exitCodePath = "/rageta/exitcode"
	contextPath  = "/rageta/context.json"
)

type Script struct {
	script   string
	stepName string
	store    secrets.Interface
}

func (s *Script) Bootstrap(_ Pipeline, next Next) (Next, error) {
	return func(ctx StepContext) (StepContext, error) {
		busybox := llb.Image("busybox:uclibc", llb.ResolveModePreferLocal)
		ctx.Build.State = ctx.Build.State.File(
			llb.Copy(busybox, "/bin/busybox", "/bin/", &llb.CopyInfo{
				CreateDestPath:                 true,
				AlwaysReplaceExistingDestPaths: false,
			}),
			llb.WithCustomNamef("copy busybox:%s → %s", "/*", "/"),
		)

		ctx.Build.State = ctx.Build.State.File(
			llb.Mkdir("/bin", 0755),
		)

		ctx.Build.State = ctx.Build.State.Run(
			llb.Shlex("/bin/busybox --install -s /bin"),
		).Root()

		script := s.script
		if err := substitute.Substitute(ctx.ToV1Beta1(), &script); err != nil {
			return ctx, err
		}
		script = strings.TrimSpace(script)

		interpreter := defaultShell
		if strings.HasPrefix(script, "#!") {
			lines := strings.SplitN(script, "\n", 2)
			interpreter = strings.TrimSpace(strings.TrimPrefix(lines[0], "#!"))
		}

		ctx.Build.State = ctx.Build.State.File(
			llb.Mkdir("/rageta", 0755),
		)

		for name, service := range ctx.Services.Status {
			netIP := net.ParseIP(service.ContainerIP)
			if netIP == nil {
				continue
			}

			ctx.Build.State = ctx.Build.State.AddExtraHost(name, netIP)
			envName := strings.ToUpper(strings.Replace(name, "-", "_", -1))
			ctx.Build.State = ctx.Build.State.AddEnv(fmt.Sprintf("SERVICE_%s", envName), service.ContainerIP)
		}

		contextJSON, err := json.MarshalIndent(ctx.ToV1Beta1(), "", "  ")
		if err != nil {
			return ctx, err
		}

		secretID := fmt.Sprintf("rageta-context-%s", s.stepName)
		s.store.AddSecret(context.Background(), secretID, contextJSON)
		ctx.Build.RunOpts = append(ctx.Build.RunOpts, llb.AddSecret(contextPath, llb.SecretID(secretID)))

		ctx.Build.State = ctx.Build.State.File(
			llb.Mkfile(scriptPath, 0755, []byte(script)),
		)

		ctx.Build.State = ctx.Build.State.Run(
			llb.Shlex(fmt.Sprintf("/bin/sh -c 'echo %s >> /rageta/ash_history'", scriptPath)),
		).Root()

		// We need the script to always exit 0 in order to get the filesystem state even in case of an error
		ctx.Build.RunOpts = append(ctx.Build.RunOpts, llb.Args([]string{
			"/bin/sh", "-c",
			fmt.Sprintf("%s -e %s; echo $? > %s", interpreter, scriptPath, exitCodePath),
		}))

		ctx, err = next(ctx)
		if err != nil {
			return ctx, err
		}

		data, err := ctx.Build.Ref.ReadFile(ctx, gwclient.ReadRequest{Filename: exitCodePath})
		if err != nil {
			return ctx, err
		}

		code, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			return ctx, fmt.Errorf("invalid exit code %q: %w", string(data), err)
		}

		if code != 0 {
			return ctx, &scriptError{
				exitCode: code,
				parent:   fmt.Errorf("exited with code %d", code),
			}
		}

		return ctx, nil
	}, nil
}

type scriptError struct {
	exitCode int
	parent   error
}

func (e *scriptError) Error() string {
	return fmt.Sprintf("script failed: %s", e.parent.Error())
}

func (e *scriptError) Unwrap() error {
	return e.parent
}

func (e *scriptError) ExitCode() int {
	return e.exitCode
}
