package progressui

import (
	"bytes"
	"container/ring"
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/moby/buildkit/client"
	digest "github.com/opencontainers/go-digest"
	"github.com/pkg/errors"
	"golang.org/x/time/rate"
)

type Display struct {
	disp display
}

type display interface {
	init(displayLimiter *rate.Limiter)
	update(ss *client.SolveStatus)
	refresh()
	done()
}

func (d Display) UpdateFrom(ctx context.Context, ch chan *client.SolveStatus) ([]client.VertexWarning, error) {
	tickerTimeout := 150 * time.Millisecond
	displayTimeout := 100 * time.Millisecond

	displayLimiter := rate.NewLimiter(rate.Every(displayTimeout), 1)
	d.disp.init(displayLimiter)
	defer d.disp.done()

	ticker := time.NewTicker(tickerTimeout)
	defer ticker.Stop()

	var warnings []client.VertexWarning
	for {
		select {
		case <-ctx.Done():
			return nil, context.Cause(ctx)
		case <-ticker.C:
			d.disp.refresh()
		case ss, ok := <-ch:
			if !ok {
				return warnings, nil
			}
			d.disp.update(ss)
			for _, w := range ss.Warnings {
				warnings = append(warnings, *w)
			}
			ticker.Reset(tickerTimeout)
		}
	}
}

type DisplayMode string

const PlainMode DisplayMode = "plain"

func NewDisplay(events, out io.Writer, mode DisplayMode) (Display, error) {
	switch mode {
	case PlainMode:
		return newPlainDisplay(events, out), nil
	default:
		return Display{}, errors.Errorf("invalid progress mode %s", mode)
	}
}

type plainDisplay struct {
	t              *trace
	printer        *textMux
	displayLimiter *rate.Limiter
}

func newPlainDisplay(events, w io.Writer) Display {
	return Display{
		disp: &plainDisplay{
			t: newTrace(events, w),
			printer: &textMux{
				w:      w,
				events: events,
			},
		},
	}
}

func (d *plainDisplay) init(displayLimiter *rate.Limiter) {
	d.displayLimiter = displayLimiter
}

func (d *plainDisplay) update(ss *client.SolveStatus) {
	if ss != nil {
		d.t.update(ss)
		if !d.displayLimiter.Allow() {
			return
		}
	}
	d.refresh()
}

func (d *plainDisplay) refresh() {
	d.printer.print(d.t)
}

func (d *plainDisplay) done() {
	d.refresh()
	d.t.printErrorLogs()
}

type trace struct {
	w             io.Writer
	events        io.Writer
	startTime     *time.Time
	localTimeDiff time.Duration
	vertexes      []*vertex
	byDigest      map[digest.Digest]*vertex
	updates       map[digest.Digest]struct{}
	groups        map[string]*vertexGroup
}

type vertex struct {
	*client.Vertex

	statuses []*status
	byID     map[string]*status
	index    int

	logs          [][]byte
	logsPartial   bool
	logsOffset    int
	logsBuffer    *ring.Ring
	prev          *client.Vertex
	events        []string
	lastBlockTime *time.Time
	count         int
	statusUpdates map[string]struct{}

	warnings   []client.VertexWarning
	warningIdx int

	intervals       map[int64]interval
	mergedIntervals []interval

	hidden bool
}

func (v *vertex) update(c int) {
	if v.count == 0 {
		now := time.Now()
		v.lastBlockTime = &now
	}
	v.count += c
}

func (v *vertex) mostRecentInterval() *interval {
	if v.isStarted() {
		ival := v.mergedIntervals[len(v.mergedIntervals)-1]
		return &ival
	}
	return nil
}

func (v *vertex) isStarted() bool {
	return len(v.mergedIntervals) > 0
}

func (v *vertex) isCompleted() bool {
	if ival := v.mostRecentInterval(); ival != nil {
		return ival.stop != nil
	}
	return false
}

type vertexGroup struct {
	*vertex
	subVtxs map[digest.Digest]client.Vertex
}

func (vg *vertexGroup) refresh() (changed, newlyStarted, newlyRevealed bool) {
	newVtx := *vg.Vertex
	newVtx.Cached = true
	alreadyStarted := vg.isStarted()
	wasHidden := vg.hidden
	for _, subVtx := range vg.subVtxs {
		if subVtx.Started != nil {
			newInterval := interval{
				start: subVtx.Started,
				stop:  subVtx.Completed,
			}
			prevInterval := vg.intervals[subVtx.Started.UnixNano()]
			if !newInterval.isEqual(prevInterval) {
				changed = true
			}
			if !alreadyStarted {
				newlyStarted = true
			}
			vg.intervals[subVtx.Started.UnixNano()] = newInterval

			if !subVtx.ProgressGroup.Weak {
				vg.hidden = false
			}
		}

		newVtx.Cached = newVtx.Cached && subVtx.Cached

		if newVtx.Error == "" {
			newVtx.Error = subVtx.Error
		} else {
			vg.hidden = false
		}
	}

	if vg.Cached != newVtx.Cached {
		changed = true
	}
	if vg.Error != newVtx.Error {
		changed = true
	}
	vg.Vertex = &newVtx

	if !vg.hidden && wasHidden {
		changed = true
		newlyRevealed = true
	}

	var ivals []interval
	for _, ival := range vg.intervals {
		ivals = append(ivals, ival)
	}
	vg.mergedIntervals = mergeIntervals(ivals)

	return changed, newlyStarted, newlyRevealed
}

type interval struct {
	start *time.Time
	stop  *time.Time
}

func (ival interval) duration() time.Duration {
	if ival.start == nil {
		return 0
	}
	if ival.stop == nil {
		return time.Since(*ival.start)
	}
	return ival.stop.Sub(*ival.start)
}

func (ival interval) isEqual(other interval) (isEqual bool) {
	return equalTimes(ival.start, other.start) && equalTimes(ival.stop, other.stop)
}

func equalTimes(t1, t2 *time.Time) bool {
	if t2 == nil {
		return t1 == nil
	}
	if t1 == nil {
		return false
	}
	return t1.Equal(*t2)
}

func mergeIntervals(intervals []interval) []interval {
	var filtered []interval
	for _, interval := range intervals {
		if interval.start != nil {
			filtered = append(filtered, interval)
		}
	}
	intervals = filtered

	if len(intervals) == 0 {
		return nil
	}

	slices.SortFunc(intervals, func(a, b interval) int {
		return a.start.Compare(*b.start)
	})

	var merged []interval
	cur := intervals[0]
	for i := 1; i < len(intervals); i++ {
		next := intervals[i]
		if cur.stop == nil {
			merged = append(merged, cur)
			return merged
		}
		if cur.stop.Before(*next.start) {
			merged = append(merged, cur)
			cur = next
			continue
		}
		if next.stop == nil {
			merged = append(merged, interval{
				start: cur.start,
				stop:  nil,
			})
			return merged
		}
		if cur.stop.After(*next.stop) || cur.stop.Equal(*next.stop) {
			continue
		}
		cur = interval{
			start: cur.start,
			stop:  next.stop,
		}
	}
	merged = append(merged, cur)
	return merged
}

type status struct {
	*client.VertexStatus
}

func newTrace(events, w io.Writer) *trace {
	return &trace{
		byDigest: make(map[digest.Digest]*vertex),
		updates:  make(map[digest.Digest]struct{}),
		w:        w,
		events:   events,
		groups:   make(map[string]*vertexGroup),
	}
}

func (t *trace) triggerVertexEvent(v *client.Vertex) {
	if v.Started == nil {
		return
	}

	var old client.Vertex
	vtx := t.byDigest[v.Digest]
	if v := vtx.prev; v != nil {
		old = *v
	}

	changed := v.Digest != old.Digest

	if v.Name != old.Name {
		changed = true
	}
	if v.Started != old.Started {
		if v.Started != nil && old.Started == nil || !v.Started.Equal(*old.Started) {
			changed = true
		}
	}
	if v.Completed != old.Completed && v.Completed != nil {
		changed = true
	}
	if v.Cached != old.Cached {
		changed = true
	}
	if v.Error != old.Error {
		changed = true
	}

	if changed {
		vtx.update(1)
		t.updates[v.Digest] = struct{}{}
	}

	t.byDigest[v.Digest].prev = v
}

func (t *trace) update(s *client.SolveStatus) {
	seenGroups := make(map[string]struct{})
	var groups []string
	for _, v := range s.Vertexes {
		if t.startTime == nil {
			t.startTime = v.Started
		}
		if v.ProgressGroup != nil {
			group, ok := t.groups[v.ProgressGroup.Id]
			if !ok {
				group = &vertexGroup{
					vertex: &vertex{
						Vertex: &client.Vertex{
							Digest: digest.Digest(v.ProgressGroup.Id),
							Name:   v.ProgressGroup.Name,
						},
						byID:          make(map[string]*status),
						statusUpdates: make(map[string]struct{}),
						intervals:     make(map[int64]interval),
						hidden:        true,
					},
					subVtxs: make(map[digest.Digest]client.Vertex),
				}
				t.groups[v.ProgressGroup.Id] = group
				t.byDigest[group.Digest] = group.vertex
			}
			if _, ok := seenGroups[v.ProgressGroup.Id]; !ok {
				groups = append(groups, v.ProgressGroup.Id)
				seenGroups[v.ProgressGroup.Id] = struct{}{}
			}
			group.subVtxs[v.Digest] = *v
			t.byDigest[v.Digest] = group.vertex
			continue
		}
		prev, ok := t.byDigest[v.Digest]
		if !ok {
			t.byDigest[v.Digest] = &vertex{
				byID:          make(map[string]*status),
				statusUpdates: make(map[string]struct{}),
				intervals:     make(map[int64]interval),
			}
		}
		t.triggerVertexEvent(v)
		if v.Started != nil && (prev == nil || !prev.isStarted()) {
			if t.localTimeDiff == 0 {
				t.localTimeDiff = time.Since(*v.Started)
			}
			t.vertexes = append(t.vertexes, t.byDigest[v.Digest])
		}
		if prev == nil || !prev.isStarted() || v.Started != nil {
			t.byDigest[v.Digest].Vertex = v
		}
		if v.Started != nil {
			t.byDigest[v.Digest].intervals[v.Started.UnixNano()] = interval{
				start: v.Started,
				stop:  v.Completed,
			}
			var ivals []interval
			for _, ival := range t.byDigest[v.Digest].intervals {
				ivals = append(ivals, ival)
			}
			t.byDigest[v.Digest].mergedIntervals = mergeIntervals(ivals)
		}
	}
	for _, groupID := range groups {
		group := t.groups[groupID]
		changed, newlyStarted, newlyRevealed := group.refresh()
		if newlyStarted {
			if t.localTimeDiff == 0 {
				t.localTimeDiff = time.Since(*group.mergedIntervals[0].start)
			}
		}
		if group.hidden {
			continue
		}
		if newlyRevealed {
			t.vertexes = append(t.vertexes, group.vertex)
		}
		if changed {
			group.update(1)
			t.updates[group.Digest] = struct{}{}
		}
	}
	for _, s := range s.Statuses {
		v, ok := t.byDigest[s.Vertex]
		if !ok {
			continue
		}
		prev, ok := v.byID[s.ID]
		if !ok {
			v.byID[s.ID] = &status{VertexStatus: s}
		}
		if s.Started != nil && (prev == nil || prev.Started == nil) {
			v.statuses = append(v.statuses, v.byID[s.ID])
		}
		v.byID[s.ID].VertexStatus = s
		v.statusUpdates[s.ID] = struct{}{}
		t.updates[v.Digest] = struct{}{}
		v.update(1)
	}
	for _, w := range s.Warnings {
		v, ok := t.byDigest[w.Vertex]
		if !ok {
			continue
		}
		v.warnings = append(v.warnings, *w)
		v.update(1)
	}
	for _, l := range s.Logs {
		v, ok := t.byDigest[l.Vertex]
		if !ok {
			continue
		}
		i := 0
		complete := split(l.Data, byte('\n'), func(dt []byte) {
			if v.logsPartial && len(v.logs) != 0 && i == 0 {
				v.logs[len(v.logs)-1] = append(v.logs[len(v.logs)-1], dt...)
			} else {
				ts := time.Duration(0)
				if ival := v.mostRecentInterval(); ival != nil {
					ts = l.Timestamp.Sub(*ival.start)
				}
				prec := 1
				sec := ts.Seconds()
				if sec < 10 {
					prec = 3
				} else if sec < 100 {
					prec = 2
				}
				v.logs = append(v.logs, fmt.Appendf(nil, "%s %s", fmt.Sprintf("%.[2]*[1]f", sec, prec), dt))
			}
			i++
		})
		v.logsPartial = !complete
		t.updates[v.Digest] = struct{}{}
		v.update(1)
	}
}

func (t *trace) printErrorLogs() {
	for _, v := range t.vertexes {
		if v.Error != "" && !strings.HasSuffix(v.Error, context.Canceled.Error()) {
			fmt.Fprintln(t.events, "------")
			fmt.Fprintf(t.events, " > %s:\n", v.Name)
			for _, l := range v.logs {
				t.w.Write(l)
				fmt.Fprintln(t.w)
			}
			if v.logsBuffer != nil {
				for range v.logsBuffer.Len() {
					if v.logsBuffer.Value != nil {
						fmt.Fprintln(t.w, string(v.logsBuffer.Value.([]byte)))
					}
					v.logsBuffer = v.logsBuffer.Next()
				}
			}
			fmt.Fprintln(t.events, "------")
		}
	}
}

func split(dt []byte, sep byte, fn func([]byte)) bool {
	if len(dt) == 0 {
		return false
	}
	for {
		if len(dt) == 0 {
			return true
		}
		idx := bytes.IndexByte(dt, sep)
		if idx == -1 {
			fn(dt)
			return false
		}
		fn(dt[:idx])
		dt = dt[idx+1:]
	}
}

