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

func WithSteps(store secrets.Interface) ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		if spec.Steps == nil {
			return nil
		}
		return &Steps{
			steps:    *spec.Steps,
			stepName: spec.Name,
			store:    store,
		}
	}
}

const (
	defaultShell   = "/bin/ash"
	contextPath    = "/rageta/context.json"
	ashHistoryPath = "/rageta/ash_history"
)

type Steps struct {
	steps    []v1beta1.Step
	stepName string
	store    secrets.Interface
}

func (s *Steps) Bootstrap(_ Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
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

		const statsReporterPath = "/rageta/stats-reporter.sh"
		const statsReporterScript = `prev=0
net_rx0=""
net_tx0=""
while true; do
  sleep 1
  if [ -f /sys/fs/cgroup/cpu.stat ]; then
    usec=$(awk '/^usage_usec/{print $2}' /sys/fs/cgroup/cpu.stat)
    mem=$(cat /sys/fs/cgroup/memory.current 2>/dev/null || echo 0)
  elif [ -f /sys/fs/cgroup/cpuacct/cpuacct.usage ]; then
    usec=$(( $(cat /sys/fs/cgroup/cpuacct/cpuacct.usage) / 1000 ))
    mem=$(cat /sys/fs/cgroup/memory/memory.usage_in_bytes 2>/dev/null || echo 0)
  else
    usec=0
    mem=0
  fi
  if [ "$prev" -gt 0 ]; then
    # Millicores, Kubernetes-style: 1000m == 1 full core-second consumed
    # during the 1s sampling window.
    cpu=$(( (usec - prev) / 1000 ))
  else
    cpu=0
  fi

  # Sum rx/tx bytes across all non-loopback interfaces. The network namespace
  # can be reused across unrelated task containers (buildkit pools netns for
  # reuse), so counters must be rebased to a per-task baseline taken on the
  # first sample rather than read as absolutes.
  net=$(awk '$2 ~ /^[0-9]+$/ && $1 !~ /^lo:/ {rx+=$2; tx+=$10} END{printf "%d %d", rx+0, tx+0}' /proc/net/dev)
  net_rx=${net% *}
  net_tx=${net#* }
  if [ -z "$net_rx0" ]; then
    net_rx0=$net_rx
    net_tx0=$net_tx
  fi

  prev=$usec
  printf '__RAGETA_STATS__ cpu=%d mem=%d net_rx=%d net_tx=%d\n' "$cpu" "$mem" "$((net_rx - net_rx0))" "$((net_tx - net_tx0))" >&2
done`

		ctx.Build.State = ctx.Build.State.File(
			llb.Mkfile(statsReporterPath, 0755, []byte(statsReporterScript)),
		)

		var scriptCmds []string
		for k, step := range s.steps {
			script := step.Script
			if err := substitute.Substitute(ctx.ToV1Beta1(), &script); err != nil {
				return ctx, err
			}
			script = strings.TrimSpace(script)

			interpreter := defaultShell
			if strings.HasPrefix(script, "#!") {
				lines := strings.SplitN(script, "\n", 2)
				interpreter = strings.TrimSpace(strings.TrimPrefix(lines[0], "#!"))
			}

			scriptPath := fmt.Sprintf("/rageta/script-%d.sh", k)
			exitCodePath := fmt.Sprintf("/rageta/exitcode-%d", k)

			ctx.Build.State = ctx.Build.State.File(
				llb.Mkfile(scriptPath, 0755, []byte(script)),
			)

			ctx.Build.State = ctx.Build.State.Run(
				llb.Shlex(fmt.Sprintf("/bin/sh -c 'echo %s >> %s'", scriptPath, ashHistoryPath)),
			).Root()

			// We need the script to always exit 0 in order to get the filesystem state even in case of an error
			scriptCmds = append(scriptCmds, fmt.Sprintf("%s -e %s; echo $? > %s", interpreter, scriptPath, exitCodePath))
		}

		// All scripts must be combined into a single Args call — multiple llb.Args in one Run only keeps the last.
		// The stats reporter runs in the background and is killed when the scripts finish.
		//
		// The user's commands are wrapped in `{ ...; } 2>&1` so that ALL of their own
		// output (whichever of stdout/stderr a given tool happens to use) lands on fd1.
		// That leaves fd2 exclusively for the stats reporter's own `__RAGETA_STATS__`
		// lines. Buildkit forwards fd1/fd2 as two independently-captured log streams,
		// so two processes never share a stream and their output can't be spliced
		// together mid-line no matter how the writes happen to interleave in time.
		userCmds := strings.Join(scriptCmds, "; ")
		wrappedCmd := fmt.Sprintf(
			"%s & __RAGETA_STATS_PID=$!; trap 'kill $__RAGETA_STATS_PID 2>/dev/null' EXIT; { %s; } 2>&1",
			statsReporterPath, userCmds,
		)
		ctx.Build.RunOpts = append(ctx.Build.RunOpts, llb.Args([]string{
			"/bin/sh", "-c",
			wrappedCmd,
		}))

		ctx, err = next(ctx)
		if err != nil {
			return ctx, err
		}

		for k := range s.steps {
			exitCodePath := fmt.Sprintf("/rageta/exitcode-%d", k)
			data, err := ctx.Build.Ref.ReadFile(ctx, gwclient.ReadRequest{Filename: exitCodePath})
			if err != nil {
				return ctx, err
			}

			code, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err != nil {
				return ctx, fmt.Errorf("invalid exit code %q from step #%d: %w", string(data), k, err)
			}

			if code != 0 {
				return ctx, &scriptError{
					exitCode: code,
					parent:   fmt.Errorf("step #%d exited with code %d", k, code),
				}
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
