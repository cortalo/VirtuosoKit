package netlist

import (
	"autorouter/common"
	"errors"
	"strings"
)

// RawLayout mirrors the layout JSON structure.
type RawLayout struct {
	Shapes    []LayoutShape    `json:"shapes"`
	Instances []LayoutInstance `json:"instances"`
}

// Layout is the expanded form of RawLayout with instances indexed by name.
type Layout struct {
	Shapes    []LayoutShape
	Instances map[string]LayoutInstance
}

type LayoutShape struct {
	Layer string        `json:"layer"`
	BBox  [2][2]float64 `json:"bbox"`
}

type TerminalInfo struct {
	Layer string        `json:"layer"`
	Bbox  [2][2]float64 `json:"bbox"` // absolute coordinates [[xLow,yLow],[xHigh,yHigh]] in µm
}

type LayoutInstance struct {
	Name      string                  `json:"name"`
	Lib       string                  `json:"lib"`
	Cell      string                  `json:"cell"`
	XY        [2]float64              `json:"xy"`
	Orient    string                  `json:"orient"`
	Terminals map[string]TerminalInfo `json:"terminals,omitempty"`
}

// Index returns a Layout with instances indexed by name.
func (r RawLayout) Index() Layout {
	instByName := make(map[string]LayoutInstance, len(r.Instances))
	for _, inst := range r.Instances {
		instByName[inst.Name] = inst
	}
	return Layout{Shapes: r.Shapes, Instances: instByName}
}

var ErrNoPRBoundary = errors.New("netlist: prBoundary shape not found in layout")

func PRBoundary(layout RawLayout) (lowerLeft, upperRight common.Point, err error) {
	for _, s := range layout.Shapes {
		if s.Layer == "prBoundary" {
			return common.Point{
					X: common.Micron(s.BBox[0][0]).ToNm(),
					Y: common.Micron(s.BBox[0][1]).ToNm(),
				}, common.Point{
					X: common.Micron(s.BBox[1][0]).ToNm(),
					Y: common.Micron(s.BBox[1][1]).ToNm(),
				}, nil
		}
	}
	return common.Point{}, common.Point{}, ErrNoPRBoundary
}

// parseOrient strips surrounding double-quotes that Virtuoso adds to orient strings.
func parseOrient(s string) string {
	s = strings.TrimPrefix(s, "\"")
	s = strings.TrimSuffix(s, "\"")
	return s
}

// transformPin applies an orientation transform to a pin bbox relative to the cell origin.
func transformPin(xLow, xHigh, yLow, yHigh common.Nm, orient string) (common.Nm, common.Nm, common.Nm, common.Nm) {
	switch orient {
	case "R90":
		return -yHigh, -yLow, xLow, xHigh
	case "R180":
		return -xHigh, -xLow, -yHigh, -yLow
	case "R270":
		return yLow, yHigh, -xHigh, -xLow
	case "MX":
		return xLow, xHigh, -yHigh, -yLow
	case "MY":
		return -xHigh, -xLow, yLow, yHigh
	case "MXR90":
		return yLow, yHigh, xLow, xHigh
	case "MYR90":
		return -yHigh, -yLow, -xHigh, -xLow
	default:
		return xLow, xHigh, yLow, yHigh
	}
}
