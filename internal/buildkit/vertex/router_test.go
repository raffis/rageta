package vertex

import (
	"sync"
	"testing"
	"time"

	"github.com/moby/buildkit/client"
	digest "github.com/opencontainers/go-digest"
)

// TestRouterBackpressure verifies Route delivers every status
// instead of silently dropping it when the sink's buffer is exhausted.
func TestRouterBackpressure(t *testing.T) {
	r := NewRouter()
	d := digest.FromString("vtx")
	ch := make(chan *client.SolveStatus) // unbuffered: every send must block for a receiver
	sink := r.Register([]digest.Digest{d}, ch)

	const n = 50
	go func() {
		for i := 0; i < n; i++ {
			r.Route(&client.SolveStatus{Vertexes: []*client.Vertex{{Digest: d}}})
		}
	}()

	received := 0
	timeout := time.After(2 * time.Second)
	for received < n {
		select {
		case <-ch:
			received++
		case <-timeout:
			t.Fatalf("received only %d/%d statuses before timeout, rest were dropped", received, n)
		}
	}

	r.Unregister([]digest.Digest{d}, sink)
}

// TestRouterDigestCollision verifies that when two sinks are
// registered under the same digest (a legitimate scenario, since digests are
// content-addressed and unrelated tasks — e.g. matrix-spawned instances of
// the same task — can share an identical LLB vertex), Route fans the status
// out to both sinks, and unregistering one never closes or removes the
// other's sink.
func TestRouterDigestCollision(t *testing.T) {
	r := NewRouter()
	d := digest.FromString("shared-vtx")

	chA := make(chan *client.SolveStatus, 1)
	sinkA := r.Register([]digest.Digest{d}, chA)

	chB := make(chan *client.SolveStatus, 1)
	sinkB := r.Register([]digest.Digest{d}, chB) // collides with sinkA on d, both are kept

	r.Route(&client.SolveStatus{Vertexes: []*client.Vertex{{Digest: d}}})
	select {
	case <-chA:
	case <-time.After(time.Second):
		t.Fatal("sinkA did not receive the routed status")
	}
	select {
	case <-chB:
	case <-time.After(time.Second):
		t.Fatal("sinkB did not receive the routed status")
	}

	// Unregistering A must not touch B's sink or its done channel, and must
	// not panic even though d no longer maps to sinkA.
	r.Unregister([]digest.Digest{d}, sinkA)

	select {
	case <-sinkB.done:
		t.Fatal("unregistering the colliding sink closed the surviving sink's done channel")
	default:
	}

	r.Route(&client.SolveStatus{Vertexes: []*client.Vertex{{Digest: d}}})
	select {
	case <-chB:
	case <-time.After(time.Second):
		t.Fatal("surviving sink did not receive the routed status")
	}

	r.Unregister([]digest.Digest{d}, sinkB)
}

// TestRouterUnregisterUnblocksRoute verifies Unregister releases
// a Route call that's blocked delivering to a sink nobody is draining
// anymore, instead of deadlocking against Route's held read lock.
func TestRouterUnregisterUnblocksRoute(t *testing.T) {
	r := NewRouter()
	d := digest.FromString("vtx")
	ch := make(chan *client.SolveStatus) // never drained
	sink := r.Register([]digest.Digest{d}, ch)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		r.Route(&client.SolveStatus{Vertexes: []*client.Vertex{{Digest: d}}})
	}()

	// Give Route a chance to block on the send before unregistering.
	time.Sleep(50 * time.Millisecond)
	r.Unregister([]digest.Digest{d}, sink)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Route stayed blocked after Unregister; deadlock")
	}
}
