package router

import "autorouter/common"

// PowerRouter routes a single power net (VDD or VSS) using a fixed M2 bus column
// at the left edge of the canvas. Each pin is connected to its nearest M3 track,
// and all M3 tracks are tied together by the vertical M2 bus.
type PowerRouter struct {
	canvas  Canvas
	m2Width int
	m2DRC   DRCSpec
	m3DRC   DRCSpec
}

func NewPowerRouter(c Canvas, m2Width int, m2DRC, m3DRC DRCSpec) *PowerRouter {
	return &PowerRouter{
		canvas:  c,
		m2Width: m2Width,
		m2DRC:   m2DRC,
		m3DRC:   m3DRC,
	}
}

// Route returns segments in the order: [m2stub_0, ..., m2stub_N-1, m3_0, ..., m3_N-1, m2bus].
// The first N segments are the M2 stubs in pin order, matching the convention of other routers.
func (r *PowerRouter) Route(pins []RoutingPin, netID int) ([]Segment, error) {
	origin := r.canvas.GetLowerLeft()
	upperRight := r.canvas.GetUpperRight()
	m3tw := r.canvas.GetTrackWidth(common.M3)
	maxTrack := (upperRight.Y-origin.Y)/m3tw - 1

	m2Stubs := make([]Segment, len(pins))
	m3Segs := make([]Segment, len(pins))

	for i, pin := range pins {
		midTrack := (pin.YLow - origin.Y) / m3tw
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

	// Full-height M2 bus at left edge connects all power M3 tracks together.
	bus, err := r.canvas.NewSeg(
		common.M2,
		Point{X: origin.X, Y: origin.Y},
		Point{X: origin.X + r.m2Width, Y: upperRight.Y},
		netID,
	)
	if err != nil {
		return nil, ErrNoPath
	}

	result := make([]Segment, 0, 2*len(pins)+1)
	result = append(result, m2Stubs...)
	result = append(result, m3Segs...)
	result = append(result, bus)
	return result, nil
}

func (r *PowerRouter) tryPinTrack(pin RoutingPin, netID, trackID int) (m3Seg Segment, m2Seg Segment, ok bool) {
	origin := r.canvas.GetLowerLeft()

	// M3 spans from left edge (to meet M2 bus) to the right edge of the pin.
	m3Start, m3End := r.m3DRC.ApplyEndExtension(origin.X, pin.XLow+r.m2Width)
	m3, err := r.canvas.NewTrack(common.M3, trackID, m3Start, m3End, netID)
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
