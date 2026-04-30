package processor

import (
	"sync"

	"github.com/moby/buildkit/client"
	digest "github.com/opencontainers/go-digest"
)

// VertexStatusRouter fans out a shared BuildKit SolveStatus channel to per-step
// channels based on vertex digest. Steps register all their LLB vertex digests
// before solving and unregister after; unregistered vertices are dropped.
type VertexStatusRouter struct {
	mu    sync.RWMutex
	sinks map[digest.Digest]chan<- *client.SolveStatus
}

func NewVertexStatusRouter() *VertexStatusRouter {
	return &VertexStatusRouter{sinks: make(map[digest.Digest]chan<- *client.SolveStatus)}
}

func (r *VertexStatusRouter) Register(digests []digest.Digest, ch chan<- *client.SolveStatus) {
	r.mu.Lock()
	for _, d := range digests {
		r.sinks[d] = ch
	}
	r.mu.Unlock()
}

func (r *VertexStatusRouter) Unregister(digests []digest.Digest) {
	r.mu.Lock()
	for _, d := range digests {
		delete(r.sinks, d)
	}
	r.mu.Unlock()
}

// Route splits a SolveStatus into per-step SolveStatus objects and sends each
// to the registered channel. Items whose vertex is not registered are dropped.
func (r *VertexStatusRouter) Route(status *client.SolveStatus) {
	r.mu.RLock()
	perCh := map[chan<- *client.SolveStatus]*client.SolveStatus{}

	for _, v := range status.Vertexes {
		if ch, ok := r.sinks[v.Digest]; ok {
			s := perCh[ch]
			if s == nil {
				s = &client.SolveStatus{}
				perCh[ch] = s
			}
			s.Vertexes = append(s.Vertexes, v)
		}
	}
	for _, st := range status.Statuses {
		if ch, ok := r.sinks[st.Vertex]; ok {
			s := perCh[ch]
			if s == nil {
				s = &client.SolveStatus{}
				perCh[ch] = s
			}
			s.Statuses = append(s.Statuses, st)
		}
	}
	for _, l := range status.Logs {
		if ch, ok := r.sinks[l.Vertex]; ok {
			s := perCh[ch]
			if s == nil {
				s = &client.SolveStatus{}
				perCh[ch] = s
			}
			s.Logs = append(s.Logs, l)
		}
	}
	for _, w := range status.Warnings {
		if ch, ok := r.sinks[w.Vertex]; ok {
			s := perCh[ch]
			if s == nil {
				s = &client.SolveStatus{}
				perCh[ch] = s
			}
			s.Warnings = append(s.Warnings, w)
		}
	}
	r.mu.RUnlock()

	for ch, s := range perCh {
		select {
		case ch <- s:
		default:
		}
	}
}
