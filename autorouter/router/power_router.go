package router

import "autorouter/common"

// PowerRouter routes a single power net (VDD or VSS) using a fixed M2 bus column
// at the left edge of the canvas. Each pin is connected to its nearest M3 track,
// and all M3 tracks are tied together by the vertical M2 bus.
type PowerRouter struct {
	canvas          Canvas
	m2Width         common.Nm
	m2DRC           DRCSpec
	m3DRC           DRCSpec
	widenNarrowPins bool
}

func NewPowerRouter(c Canvas, m2Width common.Nm, m2DRC, m3DRC DRCSpec) *PowerRouter {
	return &PowerRouter{
		canvas:  c,
		m2Width: m2Width,
		m2DRC:   m2DRC,
		m3DRC:   m3DRC,
	}
}

func (r *PowerRouter) SetWidenNarrowPins(v bool) {
	r.widenNarrowPins = v
}

// Route returns segments in the order: [m2stub_0, ..., m2stub_N-1, m3_0, ..., m3_N-1, m2bus].
// The first N segments are the M2 stubs in pin order, matching the convention of other routers.
func (r *PowerRouter) Route(pins []RoutingPin, netID int) ([]Segment, error) {
	var widenedM1s []Segment
	for i, pin := range pins {
		if r.widenNarrowPins && pin.XHigh-pin.XLow < r.m2Width {
			center := (pin.XLow + pin.XHigh) / 2
			pins[i].XLow = center - r.m2Width/2
			pins[i].XHigh = center + r.m2Width/2
			m1Seg, err := r.canvas.NewSeg(
				common.M1,
				Point{X: pins[i].XLow, Y: pin.YLow},
				Point{X: pins[i].XHigh, Y: pin.YHigh},
				netID,
			)
			if err != nil {
				return nil, err
			}
			m1Seg.NoViaUp = true
			m1Seg.NoViaDown = true
			widenedM1s = append(widenedM1s, m1Seg)
		}
	}

	origin := r.canvas.GetLowerLeft()
	upperRight := r.canvas.GetUpperRight()
	m3tw := r.canvas.GetTrackWidth(common.M3)
	maxTrack := int((upperRight.Y-origin.Y)/m3tw) - 1

	m2Stubs := make([]Segment, len(pins))
	m3Segs := make([]Segment, len(pins))

	for i, pin := range pins {
		midTrack := int((pin.YLow - origin.Y) / m3tw)
		found := false
		for delta := 0; (midTrack+delta <= maxTrack) || (midTrack-delta >= 0); delta++ {
			if m3, m2, ok := r.tryPinTrack(pin, netID, midTrack+delta); ok {
				m2Stubs[i], m3Segs[i] = m2, m3
				found = true
				break
			}
			if delta > 0 {
				if m3, m2, ok := r.tryPinTrack(pin, netID, midTrack-delta); ok {
					m2Stubs[i], m3Segs[i] = m2, m3
					found = true
					break
				}
			}
		}
		if !found {
			return nil, ErrNoPath
		}
	}

	// Find the leftmost viable M2 bus column, stepping by 2*m2Width so
	// successive power nets each get their own non-overlapping column.
	var bus Segment
	for busX := origin.X; busX+r.m2Width <= upperRight.X; busX += 2 * r.m2Width {
		candidate, err := r.canvas.NewSeg(
			common.M2,
			Point{X: busX, Y: origin.Y},
			Point{X: busX + r.m2Width, Y: upperRight.Y},
			netID,
		)
		if err == nil && r.canvas.IsPassible(candidate) {
			bus = candidate
			break
		}
	}
	if bus == (Segment{}) {
		return nil, ErrNoPath
	}

	result := make([]Segment, 0, 2*len(pins)+1+len(widenedM1s))
	result = append(result, m2Stubs...)
	result = append(result, m3Segs...)
	result = append(result, bus)
	result = append(result, widenedM1s...)
	return result, nil
}

func (r *PowerRouter) tryPinTrack(pin RoutingPin, netID, trackID int) (m3Seg Segment, m2Seg Segment, ok bool) {
	origin := r.canvas.GetLowerLeft()

	// M3 spans from left edge (to meet M2 bus) to the right edge of the pin.
	m3Start, m3End := r.m3DRC.ApplyEndExtension(origin.X, pin.XLow+r.m2Width)
	m3, err := r.canvas.NewTrack(common.M3, trackID, netID, m3Start, m3End)
	if err != nil {
		return Segment{}, Segment{}, false
	}
	m3Space := m3
	m3Space.Start, m3Space.End = r.m3DRC.ApplyMinSpaceExtension(m3.Start, m3.End)
	if !r.canvas.IsPassible(m3Space.ToSeg()) ||
		(!m3.IsFirstTrack() && r.canvas.IsOccupied(m3.PrevTrack().ToSeg())) ||
		(!m3.IsLastTrack() && r.canvas.IsOccupied(m3.NextTrack().ToSeg())) {
		return Segment{}, Segment{}, false
	}
	if !r.m3DRC.SatisfiesMinArea(m3.ToSeg()) {
		return Segment{}, Segment{}, false
	}

	// M2 stub connecting pin to M3 track.
	m2Lo, m2Hi := r.m2DRC.ApplyEndExtension(min(pin.YLow, m3.GetLower()), max(pin.YHigh, m3.GetUpper()))
	m2, err := r.canvas.NewSeg(
		common.M2,
		Point{X: pin.XLow, Y: m2Lo},
		Point{X: pin.XLow + r.m2Width, Y: m2Hi},
		netID,
	)
	if err != nil {
		return Segment{}, Segment{}, false
	}
	m2Space := m2
	m2Space.LowerLeft.Y, m2Space.UpperRight.Y = r.m2DRC.ApplyMinSpaceExtension(m2.LowerLeft.Y, m2.UpperRight.Y)
	if !r.canvas.IsPassible(m2Space) || !r.m2DRC.SatisfiesMinArea(m2) {
		return Segment{}, Segment{}, false
	}

	return m3.ToSeg(), m2, true
}
