package session

import (
	"autorouter/common"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mocks ---

type routeResult struct {
	m2From Segment
	m2To   Segment
	m3     TrackSegment
	err    error
}

type mockRouter struct {
	results []routeResult
	calls   int
}

func (m *mockRouter) Route(from, to Point, netID int) (Segment, Segment, TrackSegment, error) {
	r := m.results[m.calls]
	m.calls++
	return r.m2From, r.m2To, r.m3, r.err
}

type mockCanvas struct {
	m2Calls []Segment
	m3Calls []TrackSegment
}

func (m *mockCanvas) OccupyM2(seg Segment) error {
	m.m2Calls = append(m.m2Calls, seg)
	return nil
}

func (m *mockCanvas) OccupyM3(seg TrackSegment) error {
	m.m3Calls = append(m.m3Calls, seg)
	return nil
}

func (m *mockCanvas) GetLowerLeft() Point  { return Point{X: 0, Y: 0} }
func (m *mockCanvas) GetM3TrackWidth() int { return 100 }

// --- helpers ---

func makeNet(id, fx, fy, tx, ty int) *Net {
	return &Net{
		ID:   id,
		From: RoutingPin{XLow: fx, YLow: fy, YHigh: fy},
		To:   RoutingPin{XLow: tx, YLow: ty, YHigh: ty},
	}
}

func seg(x0, y0, x1, y1, netID int) Segment {
	return Segment{LowerLeft: Point{X: x0, Y: y0}, UpperRight: Point{X: x1, Y: y1}, NetID: netID}
}

func track(trackID, start, end, netID int) TrackSegment {
	return TrackSegment{TrackID: trackID, Start: start, End: end, NetID: netID}
}

// expectedM3Seg converts a TrackSegment to the Segment the session will produce
// using the mockCanvas geometry (LowerLeft={0,0}, trackWidth=100).
func expectedM3Seg(ts TrackSegment) Segment {
	return Segment{
		LowerLeft:  Point{X: ts.Start, Y: ts.TrackID * 100},
		UpperRight: Point{X: ts.End, Y: (ts.TrackID + 1) * 100},
		NetID:      ts.NetID,
		Layer:      common.M3,
	}
}

// --- tests ---

func TestRoute_EmptyNets_ReturnsEmptyResults(t *testing.T) {
	s := &Session{canvas: &mockCanvas{}, router: &mockRouter{}, nets: nil}
	results := s.Route()
	assert.Empty(t, results)
}

func TestRoute_SingleNet_Success_ReturnsCorrectResult(t *testing.T) {
	m2From := seg(0, 100, 10, 200, 1)
	m2To := seg(90, 100, 100, 200, 1)
	m3 := track(5, 0, 100, 1)

	router := &mockRouter{results: []routeResult{{m2From, m2To, m3, nil}}}
	canvas := &mockCanvas{}
	s := &Session{canvas: canvas, router: router, nets: []*Net{makeNet(1, 0, 100, 100, 100)}}

	results := s.Route()

	require.Len(t, results, 1)
	assert.NoError(t, results[0].Err)
	require.Len(t, results[0].Segments, 3)

	wantM2From := m2From
	wantM2From.Layer = common.M2
	wantM2To := m2To
	wantM2To.Layer = common.M2
	assert.Equal(t, wantM2From, results[0].Segments[0])
	assert.Equal(t, expectedM3Seg(m3), results[0].Segments[1])
	assert.Equal(t, wantM2To, results[0].Segments[2])
}

func TestRoute_SingleNet_Success_OccupiesCanvas(t *testing.T) {
	m2From := seg(0, 100, 10, 200, 1)
	m2To := seg(90, 100, 100, 200, 1)
	m3 := track(5, 0, 100, 1)

	router := &mockRouter{results: []routeResult{{m2From, m2To, m3, nil}}}
	canvas := &mockCanvas{}
	s := &Session{canvas: canvas, router: router, nets: []*Net{makeNet(1, 0, 100, 100, 100)}}

	s.Route()

	wantM2From := m2From
	wantM2From.Layer = common.M2
	wantM2To := m2To
	wantM2To.Layer = common.M2
	assert.Equal(t, []Segment{wantM2From, wantM2To}, canvas.m2Calls)
	assert.Equal(t, []TrackSegment{m3}, canvas.m3Calls)
}

func TestRoute_SingleNet_RouteError_SkipsOccupy(t *testing.T) {
	routeErr := errors.New("no path")
	router := &mockRouter{results: []routeResult{{err: routeErr}}}
	canvas := &mockCanvas{}
	s := &Session{canvas: canvas, router: router, nets: []*Net{makeNet(1, 0, 0, 100, 100)}}

	results := s.Route()

	require.Len(t, results, 1)
	assert.ErrorIs(t, results[0].Err, routeErr)
	assert.Empty(t, canvas.m2Calls)
	assert.Empty(t, canvas.m3Calls)
}

func TestRoute_MultipleNets_AllSucceed_OccupiesAll(t *testing.T) {
	r1 := routeResult{seg(0, 0, 10, 100, 1), seg(90, 0, 100, 100, 1), track(3, 0, 100, 1), nil}
	r2 := routeResult{seg(0, 0, 10, 200, 2), seg(90, 0, 100, 200, 2), track(7, 0, 100, 2), nil}

	router := &mockRouter{results: []routeResult{r1, r2}}
	canvas := &mockCanvas{}
	s := &Session{
		canvas: canvas,
		router: router,
		nets:   []*Net{makeNet(1, 0, 0, 100, 100), makeNet(2, 0, 0, 100, 200)},
	}

	results := s.Route()

	require.Len(t, results, 2)
	assert.NoError(t, results[0].Err)
	assert.NoError(t, results[1].Err)

	wantM2s := func(r routeResult) (Segment, Segment) {
		f, to := r.m2From, r.m2To
		f.Layer = common.M2
		to.Layer = common.M2
		return f, to
	}
	f1, to1 := wantM2s(r1)
	f2, to2 := wantM2s(r2)
	assert.Equal(t, []Segment{f1, to1, f2, to2}, canvas.m2Calls)
	assert.Equal(t, []TrackSegment{r1.m3, r2.m3}, canvas.m3Calls)
}

func TestRoute_MultipleNets_PartialFailure_OnlySuccessOccupies(t *testing.T) {
	routeErr := errors.New("no path")
	r1 := routeResult{seg(0, 0, 10, 100, 1), seg(90, 0, 100, 100, 1), track(3, 0, 100, 1), nil}
	r2 := routeResult{err: routeErr}

	router := &mockRouter{results: []routeResult{r1, r2}}
	canvas := &mockCanvas{}
	s := &Session{
		canvas: canvas,
		router: router,
		nets:   []*Net{makeNet(1, 0, 0, 100, 100), makeNet(2, 0, 0, 100, 200)},
	}

	results := s.Route()

	require.Len(t, results, 2)
	assert.NoError(t, results[0].Err)
	assert.ErrorIs(t, results[1].Err, routeErr)

	wantFrom := r1.m2From
	wantFrom.Layer = common.M2
	wantTo := r1.m2To
	wantTo.Layer = common.M2
	assert.Equal(t, []Segment{wantFrom, wantTo}, canvas.m2Calls)
	assert.Equal(t, []TrackSegment{r1.m3}, canvas.m3Calls)
}

func TestRoute_ResultsPreserveOrder(t *testing.T) {
	routeErr := errors.New("no path")
	r1 := routeResult{err: routeErr}
	r2 := routeResult{seg(0, 0, 10, 100, 2), seg(90, 0, 100, 100, 2), track(4, 0, 100, 2), nil}

	router := &mockRouter{results: []routeResult{r1, r2}}
	canvas := &mockCanvas{}
	s := &Session{
		canvas: canvas,
		router: router,
		nets:   []*Net{makeNet(1, 0, 0, 100, 100), makeNet(2, 0, 0, 100, 200)},
	}

	results := s.Route()

	require.Len(t, results, 2)
	assert.ErrorIs(t, results[0].Err, routeErr)
	assert.NoError(t, results[1].Err)
	require.Len(t, results[1].Segments, 3)

	wantFrom := r2.m2From
	wantFrom.Layer = common.M2
	assert.Equal(t, wantFrom, results[1].Segments[0])
	assert.Equal(t, expectedM3Seg(r2.m3), results[1].Segments[1])
}
