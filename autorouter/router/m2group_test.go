package router

import (
	"autorouter/canvas"
	"autorouter/common"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// spaceDRC is a NoDRC variant that adds a configurable minSpace.
type spaceDRC struct {
	common.NoDRC
	space int
}

func (d spaceDRC) ApplyMinSpaceExtension(lo, hi int) (int, int) {
	return lo - d.space, hi + d.space
}

func m2Stub(xLow, xHigh, yLow, yHigh int) Segment {
	return Segment{
		Layer:      common.M2,
		LowerLeft:  Point{X: xLow, Y: yLow},
		UpperRight: Point{X: xHigh, Y: yHigh},
	}
}

func canvasForM2Group() *canvas.TwoLayerCanvas {
	ll, ur := Point{}, Point{X: 1000, Y: 1000}
	return &canvas.TwoLayerCanvas{
		LowerLeft:  ll,
		UpperRight: ur,
		M2Storage:  canvas.NewSegmentStore(ll, ur),
		M3Storage:  canvas.NewTrackSegmentStorage(10, 100),
	}
}

// --- groupByProximity ---

func TestGroupByProximity_Empty(t *testing.T) {
	assert.Nil(t, groupByProximity(nil, 10))
}

func TestGroupByProximity_SingleStub_OneGroupNoFiller(t *testing.T) {
	stubs := []Segment{m2Stub(0, 10, 0, 100)}
	groups := groupByProximity(stubs, 10)
	require.Len(t, groups, 1)
	assert.False(t, groups[0].needsFiller())
}

func TestGroupByProximity_FarApart_TwoGroups(t *testing.T) {
	// gap = 100 - 10 = 90 >= minSpace=10 → two separate groups
	stubs := []Segment{m2Stub(0, 10, 0, 100), m2Stub(100, 110, 0, 100)}
	groups := groupByProximity(stubs, 10)
	assert.Len(t, groups, 2)
	assert.False(t, groups[0].needsFiller())
	assert.False(t, groups[1].needsFiller())
}

func TestGroupByProximity_Close_OneGroupWithFiller(t *testing.T) {
	// gap = 15 - 10 = 5 < minSpace=10 → merged into one group
	stubs := []Segment{m2Stub(0, 10, 0, 100), m2Stub(15, 25, 0, 100)}
	groups := groupByProximity(stubs, 10)
	require.Len(t, groups, 1)
	assert.True(t, groups[0].needsFiller())
}

func TestGroupByProximity_SortsByX_PreservesOriginalOrder(t *testing.T) {
	// stubs provided in reverse X order; groups should be X-sorted
	stubs := []Segment{m2Stub(100, 110, 0, 100), m2Stub(0, 10, 0, 100)}
	groups := groupByProximity(stubs, 5)
	require.Len(t, groups, 2)
	lo0, _ := groups[0].xSpan()
	lo1, _ := groups[1].xSpan()
	assert.Equal(t, 0, lo0, "first group must start at X=0 after sorting")
	assert.Equal(t, 100, lo1)
	// original slice order must be untouched
	assert.Equal(t, 100, stubs[0].LowerLeft.X)
	assert.Equal(t, 0, stubs[1].LowerLeft.X)
}

func TestGroupByProximity_MarkNoViaUp_MutatesOriginalSlice(t *testing.T) {
	stubs := []Segment{m2Stub(0, 10, 0, 100), m2Stub(5, 15, 0, 100)}
	groups := groupByProximity(stubs, 10)
	require.Len(t, groups, 1)
	groups[0].markNoViaUp()
	assert.True(t, stubs[0].NoViaUp, "mutation must propagate back through the pointer")
	assert.True(t, stubs[1].NoViaUp)
}

func TestGroupByProximity_SingleStub_MarkNoViaUp_NoMutation(t *testing.T) {
	stubs := []Segment{m2Stub(0, 10, 0, 100)}
	groups := groupByProximity(stubs, 10)
	groups[0].markNoViaUp()
	assert.False(t, stubs[0].NoViaUp, "single-stub group must not set NoViaUp")
}

func TestGroupByProximity_PanicOnNonM2(t *testing.T) {
	stubs := []Segment{{Layer: common.M3}}
	assert.Panics(t, func() { groupByProximity(stubs, 10) })
}

// --- isSingleGroup ---

func TestIsSingleGroup_True(t *testing.T) {
	stubs := []Segment{m2Stub(0, 10, 0, 100), m2Stub(5, 15, 0, 100)}
	assert.True(t, isSingleGroup(groupByProximity(stubs, 20)))
}

func TestIsSingleGroup_False(t *testing.T) {
	stubs := []Segment{m2Stub(0, 10, 0, 100), m2Stub(100, 110, 0, 100)}
	assert.False(t, isSingleGroup(groupByProximity(stubs, 10)))
}

// --- postProcessStubs ---

// Two stubs far apart (NoDRC → minSpace=0, gap=90): two single-stub groups,
// no fillers, M3 kept.
func TestPostProcessStubs_TwoGroups_KeepsM3(t *testing.T) {
	c := canvasForM2Group()
	r := NewTwoLayerRouter(c, 10, common.NoDRC{}, common.NoDRC{})

	segs := []Segment{
		m2Stub(0, 10, 50, 150),
		m2Stub(100, 110, 50, 150),
		{Layer: common.M3, LowerLeft: Point{X: 0, Y: 100}, UpperRight: Point{X: 200, Y: 200}},
	}
	result, err := r.postProcessStubs(segs, 2, 1)
	require.NoError(t, err)
	require.Len(t, result, 3, "no fillers should be added")

	m3Count := 0
	for _, s := range result {
		if s.Layer == common.M3 {
			m3Count++
		}
	}
	assert.Equal(t, 1, m3Count, "M3 must be kept when there are multiple groups")
}

// Two stubs close together (minSpace=50, gap=5): single group, M3 dropped, filler added.
// Stubs at X=[0,10] and X=[15,25]; filler must span X=[0,25] at the M3 Y range.
func TestPostProcessStubs_SingleGroup_DropsM3AndAddsFiller(t *testing.T) {
	c := canvasForM2Group()
	r := NewTwoLayerRouter(c, 10, spaceDRC{space: 50}, common.NoDRC{})

	segs := []Segment{
		m2Stub(0, 10, 50, 150),
		m2Stub(15, 25, 50, 150),
		{Layer: common.M3, LowerLeft: Point{X: 0, Y: 100}, UpperRight: Point{X: 100, Y: 200}},
	}
	result, err := r.postProcessStubs(segs, 2, 1)
	require.NoError(t, err)

	for _, s := range result {
		assert.NotEqual(t, common.M3, s.Layer, "M3 must be dropped in the single-group case")
	}
	require.Len(t, result, 3, "2 stubs + 1 filler")

	// Both original stubs must have NoViaUp set.
	assert.True(t, result[0].NoViaUp)
	assert.True(t, result[1].NoViaUp)

	// Filler: M2 spanning the full group X range at M3 Y level, NoViaDown=true.
	filler := result[2]
	assert.Equal(t, common.M2, filler.Layer)
	assert.Equal(t, 0, filler.LowerLeft.X)
	assert.Equal(t, 25, filler.UpperRight.X)
	assert.Equal(t, 100, filler.LowerLeft.Y)
	assert.Equal(t, 200, filler.UpperRight.Y)
	assert.True(t, filler.NoViaDown)
	assert.False(t, filler.NoViaUp, "filler must still be able to via up to M3 when M3 is present")
}
