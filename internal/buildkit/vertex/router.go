package vertex

import (
	"sync"

	"github.com/moby/buildkit/client"
	digest "github.com/opencontainers/go-digest"
)

// Sink is a registered destination for a set of vertex digests. done is
// closed by Unregister to release any Route call currently blocked delivering
// to ch, so a torn-down task can never cause Route to hang or send on a
// channel the owner is about to close.
type Sink struct {
	ch   chan<- *client.SolveStatus
	done chan struct{}
}

// Router fans out a shared BuildKit SolveStatus channel to per-step
// channels based on vertex digest. Tasks register all their LLB vertex digests
// before solving and unregister after; unregistered vertices are dropped.
type Router struct {
	mu    sync.RWMutex
	sinks map[digest.Digest][]*Sink
}

func NewRouter() *Router {
	return &Router{sinks: make(map[digest.Digest][]*Sink)}
}

// Register maps each digest to ch and returns a handle that must be passed to
// Unregister. Digests are content-addressed, so two unrelated tasks that
// share an identical LLB vertex (e.g. a common base image layer, or the same
// step re-run across matrix instances) can legitimately register the same
// digest; both sinks are kept and Route delivers to all of them. The
// returned handle lets Unregister act only on the caller's own sink, never
// on a sink a colliding digest also maps to.
func (r *Router) Register(digests []digest.Digest, ch chan<- *client.SolveStatus) *Sink {
	sink := &Sink{ch: ch, done: make(chan struct{})}
	r.mu.Lock()
	for _, d := range digests {
		r.sinks[d] = append(r.sinks[d], sink)
	}
	r.mu.Unlock()
	return sink
}

// Unregister releases any Route call currently blocked delivering to sink
// (so the caller can safely close its channel next) and removes sink from
// the digests it was registered under, leaving any other sink sharing that
// digest untouched. done is closed before the map is touched: Route holds
// the read lock for its full call including blocking sends, so acquiring
// the write lock first would deadlock against a Route call that's blocked
// waiting on this exact sink.
func (r *Router) Unregister(digests []digest.Digest, sink *Sink) {
	close(sink.done)

	r.mu.Lock()
	for _, d := range digests {
		sinks := r.sinks[d]
		for i, s := range sinks {
			if s == sink {
				sinks = append(sinks[:i], sinks[i+1:]...)
				break
			}
		}
		if len(sinks) == 0 {
			delete(r.sinks, d)
		} else {
			r.sinks[d] = sinks
		}
	}
	r.mu.Unlock()
}

// Route splits a SolveStatus into per-step SolveStatus objects and delivers
// each to its registered sink, blocking until delivered so a slow consumer
// applies backpressure instead of silently losing events. Items whose vertex
// is not registered are dropped. RLock is held for the full call, including
// the blocking sends, so Unregister (which takes the write lock to remove a
// sink) cannot race a close of that sink's channel with an in-flight send;
// Sink.done is what lets a blocked send bail out once Unregister has
// released the sink, instead of deadlocking against it.
func (r *Router) Route(status *client.SolveStatus) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	perSink := map[*Sink]*client.SolveStatus{}

	for _, v := range status.Vertexes {
		for _, sink := range r.sinks[v.Digest] {
			s := perSink[sink]
			if s == nil {
				s = &client.SolveStatus{}
				perSink[sink] = s
			}
			s.Vertexes = append(s.Vertexes, v)
		}
	}
	for _, st := range status.Statuses {
		for _, sink := range r.sinks[st.Vertex] {
			s := perSink[sink]
			if s == nil {
				s = &client.SolveStatus{}
				perSink[sink] = s
			}
			s.Statuses = append(s.Statuses, st)
		}
	}
	for _, l := range status.Logs {
		for _, sink := range r.sinks[l.Vertex] {
			s := perSink[sink]
			if s == nil {
				s = &client.SolveStatus{}
				perSink[sink] = s
			}
			s.Logs = append(s.Logs, l)
		}
	}
	for _, w := range status.Warnings {
		for _, sink := range r.sinks[w.Vertex] {
			s := perSink[sink]
			if s == nil {
				s = &client.SolveStatus{}
				perSink[sink] = s
			}
			s.Warnings = append(s.Warnings, w)
		}
	}

	for sink, s := range perSink {
		select {
		case sink.ch <- s:
		case <-sink.done:
		}
	}
}
