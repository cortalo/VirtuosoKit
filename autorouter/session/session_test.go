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
	segs []Segment
	err  error
}

type mockRouter struct {
	results []routeResult
	calls   int
}

func (m *mockRouter) Route(pins []RoutingPin, netID int) ([]Segment, error) {
	r := m.results[m.calls]
	m.calls++
	return r.segs, r.err
}

type mockCanvas struct {
	occupyCalls []Segment
}

func (m *mockCanvas) Occupy(seg Segment) error {
	m.occupyCalls = append(m.occupyCalls, seg)
	return nil
}

func (m *mockCanvas) GetLowerLeft() Point { return Point{X: 0, Y: 0} }

// --- helpers ---

func makeNet(id, fx, fy, tx, ty int) *Net {
	return &Net{
		ID: id,
		Pins: []RoutingPin{
			{XLow: fx, YLow: fy, YHigh: fy},
			{XLow: tx, YLow: ty, YHigh: ty},
		},
	}
}

func netlist(nets ...*Net) *Netlist {
	return &Netlist{Nets: nets}
}

// seg creates an M2 segment for use in mock router results.
func seg(x0, y0, x1, y1, netID int) Segment {
	return Segment{LowerLeft: Point{X: x0, Y: y0}, UpperRight: Point{X: x1, Y: y1}, NetID: netID, Layer: common.M2}
}

// m3Seg creates an M3 segment from a track ID using the mock canvas geometry
// (LowerLeft={0,0}, trackWidth=100).
func m3Seg(trackID, start, end, netID int) Segment {
	return Segment{
		LowerLeft:  Point{X: start, Y: trackID * 100},
		UpperRight: Point{X: end, Y: (trackID + 1) * 100},
		Layer:      common.M3,
		NetID:      netID,
	}
}

// m3FromResult finds the M3 shape in a NetResult by layer.
func m3FromResult(res NetResult) common.Shape {
	for _, s := range res.Shapes {
		if s.Layer == common.M3 {
			return s
		}
	}
	panic("no M3 shape")
}

// --- tests ---

func TestRoute_EmptyNets_ReturnsEmptyResults(t *testing.T) {
	s := &Session{canvas: &mockCanvas{}, router: &mockRouter{}, netlist: &Netlist{}, m2DRC: common.NoDRC{}, m3DRC: common.NoDRC{}}
	results := s.Route()
	assert.Empty(t, results)
}

func TestRoute_SingleNet_Success_ReturnsCorrectResult(t *testing.T) {
	m2From := seg(0, 100, 10, 200, 1)
	m2To := seg(90, 100, 100, 200, 1)
	m3 := m3Seg(5, 0, 100, 1)

	router := &mockRouter{results: []routeResult{{segs: []Segment{m2From, m2To, m3}}}}
	canvas := &mockCanvas{}
	s := &Session{canvas: canvas, router: router, netlist: netlist(makeNet(1, 0, 100, 100, 100)), m2DRC: common.NoDRC{}, m3DRC: common.NoDRC{}}

	results := s.Route()

	require.Len(t, results, 1)
	assert.NoError(t, results[0].Err)
	assert.Equal(t, m2From.ToShape(), results[0].Shapes[0])
	assert.Equal(t, m2To.ToShape(), results[0].Shapes[1])
	assert.Equal(t, m3.ToShape(), m3FromResult(results[0]))
}

func TestRoute_SingleNet_Success_OccupiesCanvas(t *testing.T) {
	m2From := seg(0, 100, 10, 200, 1)
	m2To := seg(90, 100, 100, 200, 1)
	m3 := m3Seg(5, 0, 100, 1)

	router := &mockRouter{results: []routeResult{{segs: []Segment{m2From, m2To, m3}}}}
	canvas := &mockCanvas{}
	s := &Session{canvas: canvas, router: router, netlist: netlist(makeNet(1, 0, 100, 100, 100)), m2DRC: common.NoDRC{}, m3DRC: common.NoDRC{}}

	s.Route()

	assert.Equal(t, []Segment{m2From, m2To, m3}, canvas.occupyCalls) // canvas still receives Segment
}

func TestRoute_SingleNet_RouteError_SkipsOccupy(t *testing.T) {
	routeErr := errors.New("no path")
	router := &mockRouter{results: []routeResult{{err: routeErr}}}
	canvas := &mockCanvas{}
	s := &Session{canvas: canvas, router: router, netlist: netlist(makeNet(1, 0, 0, 100, 100)), m2DRC: common.NoDRC{}, m3DRC: common.NoDRC{}}

	results := s.Route()

	require.Len(t, results, 1)
	assert.ErrorIs(t, results[0].Err, routeErr)
	assert.Empty(t, canvas.occupyCalls)
}

func TestRoute_MultipleNets_AllSucceed_OccupiesAll(t *testing.T) {
	r1 := routeResult{segs: []Segment{seg(0, 0, 10, 100, 1), seg(90, 0, 100, 100, 1), m3Seg(3, 0, 100, 1)}}
	r2 := routeResult{segs: []Segment{seg(0, 0, 10, 200, 2), seg(90, 0, 100, 200, 2), m3Seg(7, 0, 100, 2)}}

	router := &mockRouter{results: []routeResult{r1, r2}}
	canvas := &mockCanvas{}
	s := &Session{
		canvas:  canvas,
		router:  router,
		netlist: netlist(makeNet(1, 0, 0, 100, 100), makeNet(2, 0, 0, 100, 200)),
		m2DRC:   common.NoDRC{},
		m3DRC:   common.NoDRC{},
	}

	results := s.Route()

	require.Len(t, results, 2)
	assert.NoError(t, results[0].Err)
	assert.NoError(t, results[1].Err)

	wantOccupyCalls := append(append([]Segment{}, r1.segs...), r2.segs...)
	assert.Equal(t, wantOccupyCalls, canvas.occupyCalls) // canvas still receives Segment
}

func TestRoute_MultipleNets_PartialFailure_OnlySuccessOccupies(t *testing.T) {
	routeErr := errors.New("no path")
	r1 := routeResult{segs: []Segment{seg(0, 0, 10, 100, 1), seg(90, 0, 100, 100, 1), m3Seg(3, 0, 100, 1)}}
	r2 := routeResult{err: routeErr}

	router := &mockRouter{results: []routeResult{r1, r2}}
	canvas := &mockCanvas{}
	s := &Session{
		canvas:  canvas,
		router:  router,
		netlist: netlist(makeNet(1, 0, 0, 100, 100), makeNet(2, 0, 0, 100, 200)),
		m2DRC:   common.NoDRC{},
		m3DRC:   common.NoDRC{},
	}

	results := s.Route()

	require.Len(t, results, 2)
	assert.NoError(t, results[0].Err)
	assert.ErrorIs(t, results[1].Err, routeErr)
	assert.Equal(t, r1.segs, canvas.occupyCalls) // canvas still receives Segment
}

func TestRoute_ResultsPreserveOrder(t *testing.T) {
	routeErr := errors.New("no path")
	r1 := routeResult{err: routeErr}
	r2 := routeResult{segs: []Segment{seg(0, 0, 10, 100, 2), seg(90, 0, 100, 100, 2), m3Seg(4, 0, 100, 2)}}

	router := &mockRouter{results: []routeResult{r1, r2}}
	canvas := &mockCanvas{}
	s := &Session{
		canvas:  canvas,
		router:  router,
		netlist: netlist(makeNet(1, 0, 0, 100, 100), makeNet(2, 0, 0, 100, 200)),
		m2DRC:   common.NoDRC{},
		m3DRC:   common.NoDRC{},
	}

	results := s.Route()

	require.Len(t, results, 2)
	assert.ErrorIs(t, results[0].Err, routeErr)
	assert.NoError(t, results[1].Err)

	assert.Equal(t, r2.segs[0].ToShape(), results[1].Shapes[0])
	assert.Equal(t, m3Seg(4, 0, 100, 2).ToShape(), m3FromResult(results[1]))
}

func TestRoute_ViaCut_NetIDMatchesNet(t *testing.T) {
	// M2 stub covering Y=[0,300]; pin M1 bbox Y=[100,200] overlaps → via cut generated.
	m2 := seg(0, 0, 10, 300, 1)
	router := &mockRouter{results: []routeResult{{segs: []Segment{m2}}}}
	canvas := &mockCanvas{}
	via12 := common.ViaConfig{CutW: 5, CutH: 5, SpaceX: 1, SpaceY: 1}

	net := &Net{
		ID: 1,
		Pins: []RoutingPin{
			{XLow: 0, XHigh: 10, YLow: 100, YHigh: 200},
		},
	}
	s := &Session{
		canvas:  canvas,
		router:  router,
		netlist: &Netlist{Nets: []*Net{net}},
		via12:   via12,
		m2DRC:   common.NoDRC{},
		m3DRC:   common.NoDRC{},
	}

	results := s.Route()

	require.Len(t, results, 1)
	require.NoError(t, results[0].Err)

	var viaCuts []Shape
	for _, sh := range results[0].Shapes {
		if sh.Layer == common.Via12 {
			viaCuts = append(viaCuts, sh)
		}
	}
	require.NotEmpty(t, viaCuts, "expected M1-M2 via cuts to be generated")
	for _, via := range viaCuts {
		assert.Equal(t, 1, via.NetID, "via cut NetID must match net ID, not 0")
	}
}

func TestRoute_M2Pin_NoViaCut(t *testing.T) {
	// M2 segment and M2-layer pin overlap → no Via12 cut should be placed.
	m2 := seg(0, 0, 10, 300, 1)
	r := &mockRouter{results: []routeResult{{segs: []Segment{m2}}}}
	canvas := &mockCanvas{}
	via12 := common.ViaConfig{CutW: 5, CutH: 5, SpaceX: 1, SpaceY: 1}

	net := &Net{
		ID: 1,
		Pins: []RoutingPin{
			{Layer: common.M2, XLow: 0, XHigh: 10, YLow: 100, YHigh: 200},
		},
	}
	s := &Session{
		canvas:  canvas,
		router:  r,
		netlist: &Netlist{Nets: []*Net{net}},
		via12:   via12,
		m2DRC:   common.NoDRC{},
		m3DRC:   common.NoDRC{},
	}

	results := s.Route()

	require.Len(t, results, 1)
	require.NoError(t, results[0].Err)
	for _, sh := range results[0].Shapes {
		assert.NotEqual(t, common.Via12, sh.Layer, "M2-layer pin must not produce Via12 cuts")
	}
}
