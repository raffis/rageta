package processor

import (
	"context"
	"fmt"
	"time"

	"github.com/raffis/rageta/internal/buildkit/progressui"
	"github.com/raffis/rageta/internal/buildkit/vertex"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"

	"github.com/moby/buildkit/client"
	bkclient "github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	gwclient "github.com/moby/buildkit/frontend/gateway/client"
	"github.com/moby/buildkit/solver/pb"
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
	ContextState llb.State             `json:"-"`
	DebugState   *llb.State            `json:"-"`
	RunOpts      []llb.RunOption       `json:"-"`
	Mounts       []v1beta1.VolumeMount `json:"-"`
	Secrets      []SecretMount         `json:"-"`
	Ref          gwclient.Reference    `json:"-"`
	Cached       bool
	ExtraHosts   []*pb.HostIP `json:"-"`
}

// SecretMount mirrors an llb.AddSecret run option so the interactive debug
// shell can replay it: llb.RunOption is opaque once appended to
// BuildContext.RunOpts, and the debug container is built from
// gwclient.NewContainer rather than from an exec op, so it never sees those
// options.
type SecretMount struct {
	Path string
	ID   string
}

// AddSecret records a secret mount, replacing any earlier one for the same
// path so a nested task's context secret overrides its parent's instead of
// mounting twice at the same destination.
func (b *BuildContext) AddSecret(path, id string) {
	for k, secret := range b.Secrets {
		if secret.Path == path {
			b.Secrets[k].ID = id
			return
		}
	}

	b.Secrets = append(b.Secrets, SecretMount{Path: path, ID: id})
}

func (s *Build) Bootstrap(_ Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (TaskContext, error) {
		var def *llb.Definition
		var err error

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

		//state, err := res.Ref.ToState()
		//if err != nil {
		//	return ctx, fmt.Errorf("ref to state failed: %w", err)
		//}

		ctx.Build.Ref = res.Ref
		if s.builtRefs != nil {
			*s.builtRefs = append(*s.builtRefs, res.Ref)
		}
		ctx.Build.Cached = cached

		// res.Ref.ToState() starts a fresh state with none of the env/dir
		// metadata the pre-solve state had (it just wraps "read this solved
		// snapshot"), so every previously baked env var (PATH from the base
		// image, user-declared env, ...) has to be re-applied explicitly
		// here, not just PWD, or anything running against this state
		// afterwards (a further Run(), or an interactive debug shell) loses
		// them.
		//stateOpts := make([]llb.StateOption, 0, len(ctx.EnvVars.Envs)+2)
		//stateOpts = append(stateOpts, llb.Dir(ctx.Workdir.Path))
		//for k, v := range ctx.EnvVars.Envs {
		//		stateOpts = append(stateOpts, llb.AddEnv(k, v))
		//	}
		//	stateOpts = append(stateOpts, llb.AddEnv("PWD", ctx.Workdir.Path))
		//	ctx.Build.State = state.With(stateOpts...)

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
	head := headDigest(req.Definition)
	rawCh := make(chan *bkclient.SolveStatus, 16)
	sink := statusRouter.Register(digests, rawCh)

	d, err := progressui.NewDisplay(ctx.Display.Events, ctx.Display.Stdout, ctx.Display.Demuxer, progressui.PlainMode)
	if err != nil {
		statusRouter.Unregister(digests, sink)
		return nil, false, err
	}

	displayCh := make(chan *bkclient.SolveStatus, 16)
	teeDone := make(chan struct{})

	// headDone is closed as soon as the solve's final vertex is reported
	// complete, and activity is poked for every status that reaches this
	// sink; both are what drainProgress waits on once the solve RPC has
	// returned. See drainProgress for why that wait is needed at all.
	headDone := make(chan struct{})
	activity := make(chan struct{}, 1)

	go func() {
		defer close(teeDone)
		defer close(displayCh)

		headClosed := false

		pullStatuses := map[string]*bkclient.VertexStatus{}
		var lastProgressWrite time.Time
		var hadPullStatuses bool
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

			if ctx.Display.WritePullProgress != nil {
				if len(pullStatuses) > 0 {
					hadPullStatuses = true

					// throttle pull messages
					if time.Since(lastProgressWrite) >= progressWriteInterval {
						var current, total int64
						for _, v := range pullStatuses {
							current += v.Current
							total += v.Total
						}
						ctx.Display.WritePullProgress(current, total)
						lastProgressWrite = time.Now()
					}
				} else if hadPullStatuses {
					// All in-flight pulls just completed: send a final,
					// unthrottled update so the UI clears the progress bar
					// right away instead of leaving it stuck at whatever
					// value the last throttled write happened to show,
					// potentially for as long as the step keeps running.
					ctx.Display.WritePullProgress(0, 0)
					hadPullStatuses = false
				}
			}

			for _, v := range ss.Vertexes {
				cached = v.Cached

				if !headClosed && head != "" && v.Digest == head && v.Completed != nil {
					headClosed = true
					close(headDone)
				}
			}

			select {
			case activity <- struct{}{}:
			default:
			}

			displayCh <- ss
		}
	}()

	displayDone := make(chan struct{})
	go func() {
		defer close(displayDone)
		d.UpdateFrom(ctx, displayCh)

		// UpdateFrom bails out early when ctx is canceled, leaving the tee
		// goroutine blocked on a send into displayCh with nobody reading,
		// which would in turn hang the <-teeDone below forever. Keep
		// draining until the tee closes the channel so teardown always
		// completes.
		for range displayCh { //nolint:revive
		}
	}()

	res, err = gwClient.Solve(ctx, req)
	drainProgress(ctx, headDone, activity)
	statusRouter.Unregister(digests, sink)
	close(rawCh)
	<-teeDone
	<-displayDone

	return res, cached, err
}

// headDigest returns the digest of def's final vertex, i.e. the one whose
// completion marks the end of this solve's progress. The last entry in Def is
// a synthetic terminal op, so the vertex we care about is its first input.
func headDigest(def *pb.Definition) digest.Digest {
	if def == nil || len(def.Def) == 0 {
		return ""
	}

	var op pb.Op
	if err := op.UnmarshalVT(def.Def[len(def.Def)-1]); err != nil {
		return ""
	}

	if len(op.Inputs) == 0 {
		return ""
	}

	return digest.Digest(op.Inputs[0].Digest)
}

// drainProgress blocks until this solve's buildkit progress has been fully
// routed to our sink.
//
// gwClient.Solve and the progress stream are independent: Solve returns as
// soon as the solve itself is done, while the vertex statuses and log lines it
// produced are still travelling over the shared status channel that
// vertex.Router fans out. Unregistering the sink the instant Solve returns
// therefore drops everything still in flight — and for a short-lived step that
// is frequently *all* of its output, which is how a task ends up marked
// successful with a completely empty pager.
//
// Buildkit writes a vertex's logs before it records that vertex as completed,
// and the router preserves that order, so seeing the head vertex complete
// means nothing more is coming. When that never arrives (an upstream vertex
// failed, so the head never ran) fall back to waiting for the sink to go
// quiet, capped so a stalled stream can't hold the step open indefinitely.
func drainProgress(ctx context.Context, headDone <-chan struct{}, activity <-chan struct{}) {
	const (
		idlePeriod = 250 * time.Millisecond
		maxWait    = 10 * time.Second
	)

	deadline := time.NewTimer(maxWait)
	defer deadline.Stop()
	idle := time.NewTimer(idlePeriod)
	defer idle.Stop()

	for {
		select {
		case <-headDone:
			return
		case <-activity:
			if !idle.Stop() {
				select {
				case <-idle.C:
				default:
				}
			}
			idle.Reset(idlePeriod)
		case <-idle.C:
			return
		case <-deadline.C:
			return
		case <-ctx.Done():
			return
		}
	}
}
