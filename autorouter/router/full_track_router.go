package router

import (
	"autorouter/common"
	"sort"

	"github.com/samber/lo"
)

type FullTrackRouter struct {
	canvas Canvas
	m2Dir  common.Direction
	m2DRC  DRCSpec
	m3DRC  DRCSpec
}

func NewFullTrackRouter(c Canvas, m2Dir common.Direction, m2DRC, m3DRC DRCSpec) *FullTrackRouter {
	return &FullTrackRouter{canvas: c, m2Dir: m2Dir, m2DRC: m2DRC, m3DRC: m3DRC}
}

func (r *FullTrackRouter) Route(pins []RoutingPin, netID int) ([]Segment, error) {
	for _, pin := range pins {
		if !r.canvas.Inbound(Point{X: pin.XLow, Y: pin.YLow}) {
			return nil, ErrOutOfBound
		}
	}

	origin := r.canvas.GetLowerLeft()
	m2tw := r.canvas.GetTrackWidth(common.M2)
	m3tw := r.canvas.GetTrackWidth(common.M3)
	m3Dir := r.m2Dir.Perpendicular()

	m2Candidates := make([][]int, len(pins))
	for i, pin := range pins {
		m2Candidates[i] = sortedM2Tracks(pin, origin, m2tw, r.m2Dir)
		if len(m2Candidates[i]) == 0 {
			return nil, ErrPinMisaligned
		}
	}

	// midTrack is computed along the M3 direction's axis.
	var midCoord, originAlong, upperAlong common.Nm
	upperRight := r.canvas.GetUpperRight()
	switch m3Dir {
	case common.Horizontal:
		var sum common.Nm
		for _, pin := range pins {
			sum += pin.YLow
		}
		midCoord = sum / common.Nm(len(pins))
		originAlong, upperAlong = origin.Y, upperRight.Y
	case common.Vertical:
		var sum common.Nm
		for _, pin := range pins {
			sum += pin.XLow
		}
		midCoord = sum / common.Nm(len(pins))
		originAlong, upperAlong = origin.X, upperRight.X
	case common.UnknownDirection:
		panic(common.ErrUnknownDirection)
	}
	midTrack := int((midCoord - originAlong) / m3tw)
	maxTrack := int((upperAlong-originAlong)/m3tw) - 1

	for delta := 0; (midTrack+delta <= maxTrack) || (midTrack-delta >= 0); delta++ {
		if segs, ok := r.tryTrack(pins, netID, midTrack+delta, m2Candidates, origin, m2tw, m3tw, m3Dir); ok {
			return segs, nil
		}
		if delta > 0 {
			if segs, ok := r.tryTrack(pins, netID, midTrack-delta, m2Candidates, origin, m2tw, m3tw, m3Dir); ok {
				return segs, nil
			}
		}
	}
	return nil, ErrNoPath
}

// sortedM2Tracks returns all M2 track IDs that overlap the pin's cross-axis range,
// sorted by distance from the pin center (closest first).
// For M2=Vertical, the cross axis is X; for M2=Horizontal, the cross axis is Y.
func sortedM2Tracks(pin RoutingPin, origin Point, m2tw common.Nm, m2Dir common.Direction) []int {
	var crossLow, crossHigh, originCross common.Nm
	switch m2Dir {
	case common.Vertical:
		crossLow, crossHigh, originCross = pin.XLow, pin.XHigh, origin.X
	case common.Horizontal:
		crossLow, crossHigh, originCross = pin.YLow, pin.YHigh, origin.Y
	case common.UnknownDirection:
		panic(common.ErrUnknownDirection)
	}
	tMin := int((crossLow - originCross) / m2tw)
	tMax := int((crossHigh - originCross - 1) / m2tw)
	if tMax < tMin {
		return nil
	}
	tracks := make([]int, tMax-tMin+1)
	for i := range tracks {
		tracks[i] = tMin + i
	}
	center := crossLow + crossHigh - 2*originCross
	sort.Slice(tracks, func(i, j int) bool {
		di := common.Nm(2*tracks[i])*m2tw + m2tw - center
		dj := common.Nm(2*tracks[j])*m2tw + m2tw - center
		return max(di, -di) < max(dj, -dj)
	})
	return tracks
}

func (r *FullTrackRouter) tryTrack(
	pins []RoutingPin, netID, m3TrackID int,
	m2Candidates [][]int,
	origin Point, m2tw, m3tw common.Nm,
	m3Dir common.Direction,
) ([]Segment, bool) {
	// m3Lower/m3Upper are coordinates along M3's perpendicular axis (which is M2's axis).
	var m3Lower, m3Upper common.Nm
	switch m3Dir {
	case common.Horizontal:
		m3Lower = origin.Y + common.Nm(m3TrackID)*m3tw
		m3Upper = m3Lower + m3tw
	case common.Vertical:
		m3Lower = origin.X + common.Nm(m3TrackID)*m3tw
		m3Upper = m3Lower + m3tw
	case common.UnknownDirection:
		panic(common.ErrUnknownDirection)
	}
	// For each pin, pick the passible M2 track closest to the pin center.
	m2Segs := make([]TrackSegment, len(pins))
	chosenM2TrackIDs := make([]int, len(pins))
	for i, pin := range pins {
		found := false
		for _, t := range m2Candidates[i] {
			// Reject if too close to any M2 track already chosen for an earlier pin:
			// nearby same-net M2 tracks produce vias that violate via spacing DRC on
			// the M3 column. Canvas IsOccupied can't catch this because earlier pins
			// haven't been committed to the canvas yet.
			tooClose := false
			viaSpacing := r.m2DRC.ViaTrackSpacing()
			for j := 0; j < i; j++ {
				diff := t - chosenM2TrackIDs[j]
				if diff < 0 {
					diff = -diff
				}
				if diff > 0 && diff <= viaSpacing {
					tooClose = true
					break
				}
			}
			if tooClose {
				continue
			}
			// M2 runs along its direction; its start/end are the "along-axis" pin extents
			// merged with the M3 band to ensure connectivity.
			var pinAlong0, pinAlong1 common.Nm
			switch r.m2Dir {
			case common.Vertical:
				pinAlong0, pinAlong1 = pin.YLow, pin.YHigh
			case common.Horizontal:
				pinAlong0, pinAlong1 = pin.XLow, pin.XHigh
			case common.UnknownDirection:
				panic(common.ErrUnknownDirection)
			}
			var m2Start, m2End common.Nm
			if pin.Layer == common.M2 {
				extStart, extEnd := r.m2DRC.ApplyEndExtension(m3Lower, m3Upper)
				m2Start = min(extStart, pinAlong0)
				m2End = max(extEnd, pinAlong1)
			} else {
				rangeStart, rangeEnd := min(pinAlong0, m3Lower), max(pinAlong1, m3Upper)
				m2Start, m2End = r.m2DRC.ApplyEndExtension(rangeStart, rangeEnd)
			}
			m2, err := r.canvas.NewTrack(common.M2, t, netID, m2Start, m2End)
			if err != nil {
				continue
			}
			m2Space := m2
			m2Space.Start, m2Space.End = r.m2DRC.ApplyMinSpaceExtension(m2.Start, m2.End)
			if !r.canvas.IsPassible(m2Space.ToSeg()) ||
				(!m2.IsFirstTrack() && r.canvas.IsOccupied(m2.PrevTrack().ToSeg())) ||
				(!m2.IsLastTrack() && r.canvas.IsOccupied(m2.NextTrack().ToSeg())) ||
				!r.m2DRC.SatisfiesMinArea(m2.ToSeg()) {
				continue
			}
			m2Segs[i] = m2
			chosenM2TrackIDs[i] = t
			found = true
			break
		}
		if !found {
			return nil, false
		}
	}

	// M3 spans from the lowest to highest chosen M2 track along M3's axis.
	// For M3=Horizontal, M2 tracks are vertical (indexed by X), so M3 start/end are X coords.
	// For M3=Vertical, M2 tracks are horizontal (indexed by Y), so M3 start/end are Y coords.
	minM2Track := lo.Min(chosenM2TrackIDs)
	maxM2Track := lo.Max(chosenM2TrackIDs)

	if minM2Track == maxM2Track {
		result := make([]Segment, len(pins))
		var connStart, connEnd common.Nm
		for i, pin := range pins {
			var plo, phi common.Nm
			switch r.m2Dir {
			case common.Vertical:
				plo, phi = pin.YLow, pin.YHigh
			case common.Horizontal:
				plo, phi = pin.XLow, pin.XHigh
			case common.UnknownDirection:
				panic(common.ErrUnknownDirection)
			}
			if i == 0 {
				connStart, connEnd = plo, phi
			} else {
				connStart = min(connStart, plo)
				connEnd = max(connEnd, phi)
			}
			var m2Start, m2End common.Nm
			if pin.Layer == common.M2 {
				m2Start, m2End = plo, phi
			} else {
				m2Start, m2End = r.m2DRC.ApplyEndExtension(plo, phi)
			}
			m2, err := r.canvas.NewTrack(common.M2, minM2Track, netID, m2Start, m2End)
			if err != nil {
				continue
			}
			m2Space := m2
			m2Space.Start, m2Space.End = r.m2DRC.ApplyMinSpaceExtension(m2.Start, m2.End)
			if !r.canvas.IsPassible(m2Space.ToSeg()) ||
				(!m2.IsFirstTrack() && r.canvas.IsOccupied(m2.PrevTrack().ToSeg())) ||
				(!m2.IsLastTrack() && r.canvas.IsOccupied(m2.NextTrack().ToSeg())) ||
				!r.m2DRC.SatisfiesMinArea(m2.ToSeg()) {
				return nil, false
			}
			result[i] = m2.ToSeg()
		}
		conn, err := r.canvas.NewTrack(common.M2, minM2Track, netID, connStart, connEnd)
		if err != nil {
			return nil, false
		}
		return append(result, conn.ToSeg()), true
	}

	var m3Start, m3End common.Nm
	switch m3Dir {
	case common.Horizontal:
		m3Start, m3End = r.m3DRC.ApplyEndExtension(origin.X+common.Nm(minM2Track)*m2tw, origin.X+common.Nm(maxM2Track+1)*m2tw)
	case common.Vertical:
		m3Start, m3End = r.m3DRC.ApplyEndExtension(origin.Y+common.Nm(minM2Track)*m2tw, origin.Y+common.Nm(maxM2Track+1)*m2tw)
	case common.UnknownDirection:
		panic(common.ErrUnknownDirection)
	}

	m3, err := r.canvas.NewTrack(common.M3, m3TrackID, netID, m3Start, m3End)
	if err != nil {
		return nil, false
	}
	m3Space := m3
	m3Space.Start, m3Space.End = r.m3DRC.ApplyMinSpaceExtension(m3.Start, m3.End)
	if !r.canvas.IsPassible(m3Space.ToSeg()) ||
		(!m3.IsFirstTrack() && r.canvas.IsOccupied(m3.PrevTrack().ToSeg())) ||
		(!m3.IsLastTrack() && r.canvas.IsOccupied(m3.NextTrack().ToSeg())) {
		return nil, false
	}
	if !r.m3DRC.SatisfiesMinArea(m3.ToSeg()) {
		return nil, false
	}

	result := make([]Segment, len(pins)+1)
	for i, m2 := range m2Segs {
		result[i] = m2.ToSeg()
	}
	result[len(pins)] = m3.ToSeg()
	return result, true
}
