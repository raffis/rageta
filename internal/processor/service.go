package processor

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"sync"
	"syscall"
	"time"

	"github.com/moby/buildkit/client/llb"
	gwclient "github.com/moby/buildkit/frontend/gateway/client"
	"github.com/moby/buildkit/solver/pb"
	"github.com/raffis/rageta/internal/substitute"
	"github.com/raffis/rageta/internal/xio"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func WithService(gwClient gwclient.Client, teardown chan Teardown) ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		if spec.Service == nil {
			return nil
		}

		return &Service{
			command:  spec.Service.Command,
			args:     spec.Service.Args,
			gwClient: gwClient,
			teardown: teardown,
		}
	}
}

type Service struct {
	command  []string
	args     []string
	gwClient gwclient.Client
	teardown chan Teardown
	ctr      gwclient.Container
	proc     gwclient.ContainerProcess
	waitOnce sync.Once
	waitDone chan struct{}
	waitErr  error
}

type ServiceContext struct {
	NetIP net.IP
}

const serviceEntrypointPath = "/rageta/service-entrypoint.sh"
const serviceEntrypointScript = "#!" + defaultShell + `
exec "$@"
`

func (s *Service) Bootstrap(pipeline Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		command := slices.Clone(s.command)
		args := slices.Clone(s.args)

		if err := substitute.Substitute(ctx.ToV1Beta1(),
			command,
			args,
		); err != nil {
			return ctx, err
		}

		ctx.Build.State = ctx.Build.State.File(
			llb.Mkfile(serviceEntrypointPath, 0755, []byte(serviceEntrypointScript)),
		)

		ctx, err := next(ctx)
		if err != nil {
			return ctx, err
		}

		ctx, err = s.start(ctx, command, args)
		if err != nil {
			var exitCode int
			var runtimeErr ExitCode
			if errors.As(err, &runtimeErr) {
				exitCode = runtimeErr.ExitCode()
			}

			return ctx, &serviceError{
				exitCode: exitCode,
				parent:   err,
			}
		}

		return ctx, nil
	}, nil
}

func (s *Service) start(ctx TaskContext, command, args []string) (TaskContext, error) {
	ctr, err := s.gwClient.NewContainer(ctx, gwclient.NewContainerRequest{
		Mounts: []gwclient.Mount{
			{Dest: "/", MountType: pb.MountType_BIND, Ref: ctx.Build.Ref},
		},
	})
	if err != nil {
		return ctx, fmt.Errorf("create service container failed: %w", err)
	}

	ipCh := make(chan string, 1)
	ctx.Display.Demuxer.WithSink(xio.StreamIP, xio.WriterFunc(func(payload []byte) (int, error) {
		select {
		case ipCh <- string(payload):
		default:
		}
		return len(payload), nil
	}))

	cmd := append([]string{shimPath, "-stats", "-ip", serviceEntrypointPath}, append(append([]string{}, command...), args...)...)

	proc, err := ctr.Start(ctx, gwclient.StartRequest{
		Args:   cmd,
		Cwd:    ctx.Workdir.Path,
		Stdout: xio.NopWriteCloser{Writer: ctx.Display.Stdout},
		Stderr: xio.NopWriteCloser{Writer: ctx.Display.Demuxer},
	})
	if err != nil {
		_ = ctr.Release(ctx)
		return ctx, fmt.Errorf("start service failed: %w", err)
	}

	s.ctr = ctr
	s.proc = proc
	s.waitDone = make(chan struct{})

	go func() {
		s.waitOnce.Do(func() {
			s.waitErr = proc.Wait()
			close(s.waitDone)
		})
	}()

	select {
	case ip := <-ipCh:
		netIP := net.ParseIP(ip)
		if netIP == nil {
			return ctx, fmt.Errorf("invalid net ip received: %q", ip)
		}

		ctx.Service.NetIP = netIP
	case <-time.After(5 * time.Second):
		return ctx, errors.New("timed out waiting for service to report its IP")
	case <-ctx.Done():
		return ctx, ctx.Err()
	}

	s.teardown <- s.stop

	return ctx, nil
}

// stop is registered as the task's teardown closure: it signals the service
// process, escalates to SIGKILL if it doesn't exit within timeout, and
// always releases the underlying gateway container, even if the process
// never confirms it exited — teardown must never block indefinitely.
func (s *Service) stop(ctx context.Context, timeout time.Duration) error {
	_ = s.proc.Signal(ctx, syscall.SIGTERM)

	if !s.awaitDone(timeout) {
		_ = s.proc.Signal(ctx, syscall.SIGKILL)
		s.awaitDone(timeout)
	}

	return s.ctr.Release(ctx)
}

func (s *Service) awaitDone(timeout time.Duration) bool {
	select {
	case <-s.waitDone:
		return true
	case <-time.After(timeout):
		return false
	}
}

type serviceError struct {
	exitCode int
	parent   error
}

func (e *serviceError) Error() string {
	return fmt.Sprintf("service failed: %s", e.parent.Error())
}

func (e *serviceError) Unwrap() error {
	return e.parent
}

func (e *serviceError) ExitCode() int {
	return e.exitCode
}
