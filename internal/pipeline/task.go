package pipeline

import (
	"sync"

	"github.com/raffis/rageta/internal/processor"
)

type pipelineTask struct {
	processors []processor.Bootstraper
	name       string
	pipeline   *pipeline
	dependsOn  []dependsOnRef
	labels     map[string]string
	claims     map[string]processor.TaskClaim
	pending    map[string]int
	mu         sync.Mutex
}

func (p *pipelineTask) Processors() []processor.Bootstraper {
	return p.processors
}

func (p *pipelineTask) Name() string {
	return p.name
}

func (p *pipelineTask) Entrypoint() (processor.Next, error) {
	return processor.Chain(p.pipeline, p.processors...)
}

func (p *pipelineTask) Claim(ctx processor.TaskContext) (processor.TaskClaim, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.claims == nil {
		p.claims = make(map[string]processor.TaskClaim)
	}

	key := ctx.Namespace()
	if claim, ok := p.claims[key]; ok {
		return claim, false
	}

	claim := &taskClaim{
		done: make(chan struct{}),
		ctx:  ctx,
	}
	p.claims[key] = claim

	return claim, true
}

// Ready decrements the number of unmet dependencies of this task for the
// given run and reports whether that was the last one, i.e. whether all of
// this task's dependencies have now completed and it may be launched.
func (p *pipelineTask) Ready(ctx processor.TaskContext) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.pending == nil {
		p.pending = make(map[string]int)
	}

	key := ctx.Namespace()
	remaining, ok := p.pending[key]
	if !ok {
		remaining = len(p.dependsOn)
	}

	remaining--
	p.pending[key] = remaining

	return remaining <= 0
}

type taskClaim struct {
	done chan struct{}
	ctx  processor.TaskContext
	err  error
}

func (r *taskClaim) Release(ctx processor.TaskContext, err error) {
	r.ctx = ctx
	r.err = err
	close(r.done)
}

func (r *taskClaim) Wait() (processor.TaskContext, error) {
	<-r.done
	return r.ctx, r.err
}
