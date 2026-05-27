package router

import (
	"autorouter/common"
	"sort"

	"github.com/samber/lo"
)

type Point = common.Point
type Segment = common.Segment
type TrackSegment = common.TrackSegment
type RoutingPin = common.RoutingPin

type Canvas interface {
	Inbound(p Point) bool
	IsPassible(seg Segment) bool
	IsOccupied(seg Segment) bool
	GetLowerLeft() Point
	GetUpperRight() Point
	GetTrackWidth(layer common.Layer) int
	NewTrack(layer common.Layer, trackID, start, end, netID int) (TrackSegment, error)
	NewSeg(layer common.Layer, ll, ur Point, netID int) (Segment, error)
}

type DRCSpec = common.DRCSpec

type TwoLayerRouter struct {
	canvas          Canvas
	m2Width         int
	m2DRC           DRCSpec
	m3DRC           DRCSpec
	widenNarrowPins bool
}

func NewTwoLayerRouter(c Canvas, m2Width int, m2DRC, m3DRC DRCSpec) *TwoLayerRouter {
	return &TwoLayerRouter{canvas: c, m2Width: m2Width, m2DRC: m2DRC, m3DRC: m3DRC}
}

func (r *TwoLayerRouter) SetWidenNarrowPins(v bool) {
	r.widenNarrowPins = v
}

func (r *TwoLayerRouter) maybeWidenPin(pin *RoutingPin, widenedM1s *[]Segment, netID int) error {
	if !r.widenNarrowPins || pin.XHigh-pin.XLow >= r.m2Width {
		return nil
	}
	center := (pin.XLow + pin.XHigh) / 2
	pin.XLow = center - r.m2Width/2
	pin.XHigh = center + r.m2Width/2
	m1Seg, err := r.canvas.NewSeg(
		common.M1,
		Point{X: pin.XLow, Y: pin.YLow},
		Point{X: pin.XHigh, Y: pin.YHigh},
		netID,
	)
	if err != nil {
		return err
	}
	m1Seg.NoViaUp = true
	m1Seg.NoViaDown = true
	*widenedM1s = append(*widenedM1s, m1Seg)
	return nil
}

func (r *TwoLayerRouter) Route(pins []RoutingPin, netID int) ([]Segment, error) {
	var widenedM1s []Segment
	for i := range pins {
		if !r.canvas.Inbound(Point{X: pins[i].XLow, Y: pins[i].YLow}) {
			return nil, ErrOutOfBound
		}
		if err := r.maybeWidenPin(&pins[i], &widenedM1s, netID); err != nil {
			return nil, err
		}
	}

	sumY := 0
	for _, pin := range pins {
		sumY += pin.YLow
	}
	midY := sumY / len(pins)

	lowerLeft := r.canvas.GetLowerLeft()
	upperRight := r.canvas.GetUpperRight()
	tw := r.canvas.GetTrackWidth(common.M3)
	midTrack := (midY - lowerLeft.Y) / tw
	maxTrack := (upperRight.Y-lowerLeft.Y)/tw - 1

	tryAndProcess := func(trackID int) ([]Segment, bool) {
		segs, ok := r.tryTrack(pins, netID, trackID)
		if !ok {
			return nil, false
		}
		result, err := r.postProcessStubs(segs, len(pins), netID)
		if err != nil {
			return nil, false
		}
		return result, true
	}

	for delta := 0; (midTrack+delta <= maxTrack) || (midTrack-delta >= 0); delta++ {
		if segs, ok := tryAndProcess(midTrack + delta); ok {
			return append(segs, widenedM1s...), nil
		}
		if delta > 0 {
			if segs, ok := tryAndProcess(midTrack - delta); ok {
				return append(segs, widenedM1s...), nil
			}
		}
	}
	return nil, ErrNoPath
}

func (r *TwoLayerRouter) m2Passible(seg Segment) bool {
	sp := seg
	sp.LowerLeft.X, sp.UpperRight.X = r.m2DRC.ApplyMinSpaceExtension(seg.LowerLeft.X, seg.UpperRight.X)
	sp.LowerLeft.Y, sp.UpperRight.Y = r.m2DRC.ApplyMinSpaceExtension(seg.LowerLeft.Y, seg.UpperRight.Y)
	return r.canvas.IsPassible(sp) && r.m2DRC.SatisfiesMinArea(seg)
}

func (r *TwoLayerRouter) m3Passible(ts TrackSegment) bool {
	sp := ts
	sp.Start, sp.End = r.m3DRC.ApplyMinSpaceExtension(ts.Start, ts.End)
	return r.canvas.IsPassible(sp.ToSeg()) &&
		(ts.IsFirstTrack() || !r.canvas.IsOccupied(ts.PrevTrack().ToSeg())) &&
		(ts.IsLastTrack() || !r.canvas.IsOccupied(ts.NextTrack().ToSeg()))
}

func (r *TwoLayerRouter) tryTrack(pins []RoutingPin, netID, trackID int) ([]Segment, bool) {
	minX := lo.Min(lo.Map(pins, func(p RoutingPin, _ int) int { return p.XLow }))
	maxXRight := lo.Max(lo.Map(pins, func(p RoutingPin, _ int) int { return max(p.XHigh, p.XLow+r.m2Width) }))

	m3Lo, m3Hi := r.m3DRC.ApplyEndExtension(minX, maxXRight)
	m3, err := r.canvas.NewTrack(common.M3, trackID, m3Lo, m3Hi, netID)
	if err != nil {
		return nil, false
	}
	if !r.m3Passible(m3) {
		return nil, false
	}

	step := r.m2Width / 2
	if step == 0 {
		step = 1
	}

	m2Segs := make([]Segment, len(pins))
	for i, pin := range pins {
		m2Lo, m2Hi, narrowErr := r.pinM2Bounds(pin, m3)
		m2Lo, m2Hi = r.m2DRC.ApplyEndExtension(m2Lo, m2Hi)

		xMax := pin.XLow // narrow pin: only try XLow
		if narrowErr == nil {
			xMax = pin.XHigh - r.m2Width
		}

		found := false
		for xLow := pin.XLow; xLow <= xMax; xLow += step {
			seg, segErr := r.canvas.NewSeg(
				common.M2,
				Point{X: xLow, Y: m2Lo},
				Point{X: xLow + r.m2Width, Y: m2Hi},
				netID,
			)
			if segErr != nil || !r.m2Passible(seg) {
				continue
			}
			m2Segs[i] = seg
			found = true
			break
		}
		if !found {
			return nil, false
		}
	}

	return append(m2Segs, m3.ToSeg()), true
}

func (r *TwoLayerRouter) postProcessStubs(segs []Segment, nPins, netID int) ([]Segment, error) {
	_, minSpace := r.m2DRC.ApplyMinSpaceExtension(0, 0)
	m3 := segs[nPins]
	groups := groupByProximity(segs[:nPins], minSpace)
	var fillers []Segment
	for _, g := range groups {
		g.markNoViaUp()
		if g.needsFiller() {
			f, err := g.filler(r.canvas, m3.LowerLeft.Y, m3.UpperRight.Y, netID)
			if err != nil {
				return nil, err
			}
			fillers = append(fillers, f)
		}
	}
	if isSingleGroup(groups) {
		// All stubs are clustered; the filler M2 connects them directly, M3 is not needed.
		return append(segs[:nPins:nPins], fillers...), nil
	}
	return append(segs, fillers...), nil
}

func isSingleGroup(groups []m2Group) bool { return len(groups) == 1 }

// m2Group holds pointers into the original stubs slice, sorted by X.
// Pointers allow markNoViaUp to mutate the original segments in place
// without disturbing the pin-order of the source slice.
type m2Group struct {
	stubs []*Segment
}

func (g m2Group) needsFiller() bool { return len(g.stubs) > 1 }

func (g m2Group) xSpan() (xLo, xHi int) {
	return g.stubs[0].LowerLeft.X, g.stubs[len(g.stubs)-1].UpperRight.X
}

func (g m2Group) markNoViaUp() {
	if len(g.stubs) < 2 {
		return
	}
	for _, s := range g.stubs {
		s.NoViaUp = true
	}
}

// filler creates an M2 bar spanning the group's X range at the M3 track Y level,
// to be used as the single via contact point to M3. NoViaDown is set so the filler
// does not attempt a via back down to M1.
func (g m2Group) filler(c Canvas, m3YLo, m3YHi, netID int) (Segment, error) {
	xLo, xHi := g.xSpan()
	seg, err := c.NewSeg(common.M2, Point{X: xLo, Y: m3YLo}, Point{X: xHi, Y: m3YHi}, netID)
	if err != nil {
		return Segment{}, err
	}
	seg.NoViaDown = true
	return seg, nil
}

// groupByProximity partitions m2Stubs into groups where consecutive stubs
// (sorted by X) have a gap smaller than minSpace. Stubs within a group are
// too close to carry independent vias to M3 and will share a filler bar.
func groupByProximity(m2Stubs []Segment, minSpace int) []m2Group {
	if len(m2Stubs) == 0 {
		return nil
	}
	ptrs := make([]*Segment, len(m2Stubs))
	for i := range m2Stubs {
		ptrs[i] = &m2Stubs[i]
		if ptrs[i].Layer != common.M2 {
			panic("groupByProximity: non-M2 segment passed in")
		}
	}
	sort.Slice(ptrs, func(i, j int) bool {
		return ptrs[i].LowerLeft.X < ptrs[j].LowerLeft.X
	})

	groups := []m2Group{{stubs: []*Segment{ptrs[0]}}}
	for _, ptr := range ptrs[1:] {
		cur := &groups[len(groups)-1]
		rightEdge := cur.stubs[len(cur.stubs)-1].UpperRight.X
		if ptr.LowerLeft.X-rightEdge < minSpace {
			cur.stubs = append(cur.stubs, ptr)
		} else {
			groups = append(groups, m2Group{stubs: []*Segment{ptr}})
		}
	}
	return groups
}

// pinM2Bounds returns the raw (before end-extension) Y range for the M2 stub
// connecting pin to m3, and errPinTooNarrow if pin.XHigh-pin.XLow < m2Width
// (caller should not attempt to slide the stub in that case).
func (r *TwoLayerRouter) pinM2Bounds(pin RoutingPin, m3 TrackSegment) (lo, hi int, err error) {
	if pin.XHigh-pin.XLow < r.m2Width {
		err = errPinTooNarrow
	}
	minOv := r.m2DRC.MinPinOverlap()
	if pin.MinOverlap && minOv > 0 {
		pinCenterY := (pin.YLow + pin.YHigh) / 2
		m3CenterY := (m3.GetLower() + m3.GetUpper()) / 2
		if pinCenterY <= m3CenterY {
			return min(m3.GetLower(), max(pin.YLow, min(pin.YHigh, m3.GetUpper())-minOv)), m3.GetUpper(), err
		}
		return m3.GetLower(), max(m3.GetUpper(), min(pin.YHigh, max(pin.YLow, m3.GetLower())+minOv)), err
	}
	return min(pin.YLow, m3.GetLower()), max(pin.YHigh, m3.GetUpper()), err
}
