package processor

import (
	"context"
	"fmt"

	"github.com/raffis/rageta/internal/utils/progressui"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"

	bkclient "github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	gwclient "github.com/moby/buildkit/frontend/gateway/client"
	digest "github.com/opencontainers/go-digest"
)

func WithBuild(gwClient gwclient.Client, statusRouter *VertexStatusRouter, cacheImports []gwclient.CacheOptionsEntry, noCache bool, builtRefs *[]gwclient.Reference) ProcessorBuilder {
	return func(spec *v1beta1.Task) Bootstraper {
		if spec.Steps == nil {
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

type Build struct {
	gwClient     gwclient.Client
	statusRouter *VertexStatusRouter
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
	State        llb.State
	ContextState *llb.State // initial inherited state, set when entering a sub-pipeline via inherit
	RunOpts      []llb.RunOption
	Ref          gwclient.Reference
	Cached       bool
}

func (s *Build) Bootstrap(_ Pipeline, next Next) (Next, error) {
	return func(ctx TaskContext) (outCtx TaskContext, outErr error) {
		if s.noCache {
			ctx.Build.RunOpts = append(ctx.Build.RunOpts, llb.IgnoreCache)
		}

		if ctx.Build.RunOpts == nil {
			return next(ctx)
		}

		exec := ctx.Build.State.Run(ctx.Build.RunOpts...)
		def, err := exec.Root().Marshal(ctx)
		if err != nil {
			return ctx, fmt.Errorf("marshal root failed: %w", err)
		}

		var allCached bool
		if s.statusRouter != nil {
			if _, herr := def.Head(); herr == nil {
				var digests []digest.Digest
				for _, dt := range def.ToPB().Def {
					digests = append(digests, digest.FromBytes(dt))
				}
				rawCh := make(chan *bkclient.SolveStatus, 16)
				s.statusRouter.Register(digests, rawCh)

				d, derr := progressui.NewDisplay(ctx.Events.Dev, ctx.Display.Stdout, progressui.PlainMode)
				if derr != nil {
					s.statusRouter.Unregister(digests)
					return ctx, derr
				}

				displayCh := make(chan *bkclient.SolveStatus, 16)
				teeDone := make(chan struct{})
				go func() {
					defer close(teeDone)
					defer close(displayCh)
					for ss := range rawCh {
						for _, v := range ss.Statuses {
							fmt.Printf("BUILD %s %s %#v %d %d\n", v.ID, v.Name, v.Completed, v.Current, v.Total)
						}

						for _, v := range ss.Vertexes {
							allCached = v.Cached
						}
						displayCh <- ss
					}
				}()

				displayDone := make(chan struct{})
				go func() {
					defer close(displayDone)
					d.UpdateFrom(ctx, displayCh)
				}()
				defer func() {
					s.statusRouter.Unregister(digests)
					close(rawCh)
					<-teeDone
					<-displayDone
					outCtx.Build.Cached = allCached
				}()
			}
		}

		ref, err := s.solve(ctx, def)
		if err != nil {
			return ctx, fmt.Errorf("solve failed: %w", err)
		}

		state, err := ref.ToState()
		if err != nil {
			return ctx, fmt.Errorf("ref to state failed: %w", err)
		}

		ctx.Build.Ref = ref
		if s.builtRefs != nil {
			*s.builtRefs = append(*s.builtRefs, ref)
		}
		ctx.Build.State = state
		ctx.Build.State = state.With(llb.Dir(ctx.Workdir.Path))

		outCtx, outErr = next(ctx)
		return
	}, nil
}

func (s *Build) solve(ctx context.Context, def *llb.Definition) (gwclient.Reference, error) {
	res, err := s.gwClient.Solve(ctx, gwclient.SolveRequest{
		Definition:   def.ToPB(),
		CacheImports: s.cacheImports,
		Evaluate:     true,
	})
	if err != nil {
		return nil, err
	}
	return res.Ref, nil
}
