package netlist

import (
	"fmt"
	"math"

	"autorouter/common"
)

// BuildEscapeShapes generates M1 escape wires and poly contact (CC) cuts for
// every escape-cell instance in the layout. For each pin that has both an
// escape target defined in cells.toml and a terminal entry in the layout JSON,
// it delegates to escapePathShapes to produce the PC shape, M1 metal, and
// Contact cuts.
//
// contactVC is the via config for poly contacts (CC/CNT cuts).
// Instances whose cell is not marked escape are silently skipped.
// Pins absent from cells.toml are silently skipped.
func BuildEscapeShapes(layout Layout, db PinDB, contactVC common.ViaConfig) ([]common.Shape, error) {
	var shapes []common.Shape
	for _, inst := range layout.Instances {
		isEscape, err := db.IsEscapeCell(inst.Lib, inst.Cell)
		if err != nil || !isEscape {
			continue
		}
		instX := int(math.Round(inst.XY[0] * 1000))
		instY := int(math.Round(inst.XY[1] * 1000))
		orient := parseOrient(inst.Orient)

		for pinName, term := range inst.Terminals {
			xLow, xHigh, yLow, yHigh, _, qerr := db.Query(inst.Lib, inst.Cell, pinName)
			if qerr != nil {
				continue
			}
			txLow, txHigh, tyLow, tyHigh := transformPin(xLow, xHigh, yLow, yHigh, orient)
			m1Pin := common.Shape{
				Layer:      common.M1,
				LowerLeft:  common.Point{X: instX + txLow, Y: instY + tyLow},
				UpperRight: common.Point{X: instX + txHigh, Y: instY + tyHigh},
			}

			termLayer, lerr := common.ParseLayer(term.Layer)
			if lerr != nil {
				return nil, fmt.Errorf("netlist: escape instance %q pin %q: terminal layer %q: %w",
					inst.Name, pinName, term.Layer, lerr)
			}

			if termLayer == common.PC {
				pcShape := common.Shape{
					Layer:      common.PC,
					LowerLeft:  common.Point{X: int(math.Round(term.Bbox[0][0] * 1000)), Y: int(math.Round(term.Bbox[0][1] * 1000))},
					UpperRight: common.Point{X: int(math.Round(term.Bbox[1][0] * 1000)), Y: int(math.Round(term.Bbox[1][1] * 1000))},
				}
				shapes = append(shapes, escapePathShapes(pcShape, m1Pin, contactVC)...)
			} else {
				shapes = append(shapes, m1Pin)
			}
		}
	}
	return shapes, nil
}

// escapePathShapes returns the PC shape, M1 escape metal, and Contact cuts
// that connect pcShape (poly terminal, absolute coords) to m1Pin (cells.toml
// escape target, already transformed and offset to absolute coords).
//
// If pcShape completely contains m1Pin, only m1Pin and its contacts are added.
// Otherwise an L-shaped M1 path is built:
//   - a vertical segment at PC's X width, spanning vertically to include both
//     PC and pin (so the two segments share an overlapping corner)
//   - a horizontal segment at pin's Y height, spanning from the PC column to
//     fully cover pin in X
//
// Contacts are placed at the overlap of pcShape and the vertical segment.
func escapePathShapes(pcShape, m1Pin common.Shape, contactVC common.ViaConfig) []common.Shape {
	pcX0, pcY0 := pcShape.LowerLeft.X, pcShape.LowerLeft.Y
	pcX1, pcY1 := pcShape.UpperRight.X, pcShape.UpperRight.Y
	pinX0, pinY0 := m1Pin.LowerLeft.X, m1Pin.LowerLeft.Y
	pinX1, pinY1 := m1Pin.UpperRight.X, m1Pin.UpperRight.Y

	out := []common.Shape{pcShape}

	// Case 1: PC completely contains pin — just emit pin + contacts.
	if pcX0 <= pinX0 && pcX1 >= pinX1 && pcY0 <= pinY0 && pcY1 >= pinY1 {
		out = append(out, m1Pin)
		return common.PlaceViaCuts(out, pcShape, m1Pin, contactVC, common.Contact, 0)
	}

	// Case 2+3: L-shaped M1 path.
	// Vertical M1 segment at PC's X width; its Y range covers both PC and pin
	// so that it overlaps PC (for contacts) and shares area with horizSeg.
	var vertY0, vertY1 int
	switch {
	case pcY0 > pinY1: // PC is above pin
		vertY0, vertY1 = pinY0, pcY1
	case pcY1 < pinY0: // PC is below pin
		vertY0, vertY1 = pcY0, pinY1
	default: // Y ranges already overlap
		vertY0, vertY1 = pcY0, pcY1
	}
	vertSeg := common.Shape{
		Layer:      common.M1,
		LowerLeft:  common.Point{X: pcX0, Y: vertY0},
		UpperRight: common.Point{X: pcX1, Y: vertY1},
	}

	// Horizontal M1 segment at pin's Y height, spanning from PC column to cover pin.
	horizSeg := common.Shape{
		Layer:      common.M1,
		LowerLeft:  common.Point{X: min(pcX0, pinX0), Y: pinY0},
		UpperRight: common.Point{X: max(pcX1, pinX1), Y: pinY1},
	}

	out = append(out, vertSeg, horizSeg)
	return common.PlaceViaCuts(out, pcShape, vertSeg, contactVC, common.Contact, 0)
}
