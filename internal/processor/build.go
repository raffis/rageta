package processor

import (
	"fmt"
	"time"

	"github.com/raffis/rageta/internal/buildkit/progressui"
	"github.com/raffis/rageta/internal/buildkit/vertex"
	"github.com/raffis/rageta/internal/processor/shimbin"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"

	"github.com/moby/buildkit/client"
	bkclient "github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	gwclient "github.com/moby/buildkit/frontend/gateway/client"
	digest "github.com/opencontainers/go-digest"
)

func WithBuild(gwClient gwclient.Client, statusRouter vertexRouter, cacheImports []gwclient.CacheOptionsEntry, noCache bool, builtRefs *[]gwclient.Reference) ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		if spec.Steps == nil && spec.Service == nil {
			return nil
		}

		if statusRouter == nil {
			return nil
		}

		return &Build{
			gwClient:     gwClient,
			statusRouter: statusRouter,
			cacheImports: cacheImports,
			noCache:      noCache,
			builtRefs:    builtRefs,
		}
	}
}

type vertexRouter interface {
	Register(digests []digest.Digest, ch chan<- *client.SolveStatus) *vertex.Sink
	Unregister(digests []digest.Digest, sink *vertex.Sink)
}

type Build struct {
	gwClient     gwclient.Client
	statusRouter vertexRouter
	cacheImports []gwclient.CacheOptionsEntry
	noCache      bool
	builtRefs    *[]gwclient.Reference
}

func newBuildContext() BuildContext {
	return BuildContext{
		State: llb.Scratch(),
	}
}

type BuildContext struct {
	State        llb.State             `json:"-"`
	ContextState *llb.State            `json:"-"`
	RunOpts      []llb.RunOption       `json:"-"`
	Mounts       []v1beta1.VolumeMount `json:"-"`
	Ref          gwclient.Reference    `json:"-"`
	Cached       bool
}

func (s *Build) Bootstrap(_ Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		var def *llb.Definition
		var err error

		ctx.Build.State = bakeBusybox(ctx.Build.State)
		ctx.Build.State = bakeShim(ctx.Build.State)

		def, err = ctx.Build.State.Marshal(ctx)
		if err != nil {
			return ctx, fmt.Errorf("marshal root failed: %w", err)
		}

		if _, err := def.Head(); err != nil {
			return ctx, err
		}

		res, cached, err := solveWithProgress(ctx, s.gwClient, s.statusRouter, gwclient.SolveRequest{
			Definition:   def.ToPB(),
			CacheImports: s.cacheImports,
			Evaluate:     true,
		})
		if err != nil {
			return ctx, fmt.Errorf("solve failed: %w", err)
		}

		state, err := res.Ref.ToState()
		if err != nil {
			return ctx, fmt.Errorf("ref to state failed: %w", err)
		}

		ctx.Build.Ref = res.Ref
		if s.builtRefs != nil {
			*s.builtRefs = append(*s.builtRefs, res.Ref)
		}
		ctx.Build.Cached = cached
		ctx.Build.State = state.With(llb.Dir(ctx.Workdir.Path), llb.AddEnv("PWD", ctx.Workdir.Path))

		return next(ctx)
	}, nil
}

// solveWithProgress solves req, streaming its buildkit progress through
// statusRouter/progressui for the duration of the solve. Used by both Build
// (solving a Run() exec's resulting state) and Service (solving the service's
// base image state directly) so the digest-registration and display-piping
// logic isn't duplicated between the two execution models.
func solveWithProgress(ctx TaskContext, gwClient gwclient.Client, statusRouter vertexRouter, req gwclient.SolveRequest) (res *gwclient.Result, cached bool, err error) {
	var digests []digest.Digest
	for _, dt := range req.Definition.Def {
		digests = append(digests, digest.FromBytes(dt))
	}
	rawCh := make(chan *bkclient.SolveStatus, 16)
	sink := statusRouter.Register(digests, rawCh)

	d, err := progressui.NewDisplay(ctx.Display.Events, ctx.Display.Stdout, ctx.Display.Demuxer, progressui.PlainMode)
	if err != nil {
		statusRouter.Unregister(digests, sink)
		return nil, false, err
	}

	displayCh := make(chan *bkclient.SolveStatus, 16)
	teeDone := make(chan struct{})
	go func() {
		defer close(teeDone)
		defer close(displayCh)

		pullStatuses := map[string]*bkclient.VertexStatus{}
		var lastProgressWrite time.Time
		const progressWriteInterval = 100 * time.Millisecond
		for ss := range rawCh {
			for _, v := range ss.Statuses {
				if v.Total <= 0 {
					continue
				}

				if v.Completed != nil {
					delete(pullStatuses, v.ID)
				} else {
					pullStatuses[v.ID] = v
				}
			}

			// throttle pull messages
			if ctx.Display.WritePullProgress != nil && len(pullStatuses) > 0 &&
				time.Since(lastProgressWrite) >= progressWriteInterval {
				var current, total int64
				for _, v := range pullStatuses {
					current += v.Current
					total += v.Total
				}
				ctx.Display.WritePullProgress(current, total)
				lastProgressWrite = time.Now()
			}

			for _, v := range ss.Vertexes {
				cached = v.Cached
			}
			displayCh <- ss
		}
	}()

	displayDone := make(chan struct{})
	go func() {
		defer close(displayDone)
		d.UpdateFrom(ctx, displayCh)
	}()

	res, err = gwClient.Solve(ctx, req)
	statusRouter.Unregister(digests, sink)
	close(rawCh)
	<-teeDone
	<-displayDone

	return res, cached, err
}

const shimPath = "/rageta/shim"

func bakeShim(state llb.State) llb.State {
	state = state.File(
		llb.Mkdir("/rageta", 0755),
	)

	return state.File(
		llb.Mkfile(shimPath, 0755, shimbin.Binary),
	)
}
