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
func BuildEscapeShapes(layout RawLayout, db PinDB, contactVC common.ViaConfig) ([]common.Shape, error) {
	var shapes []common.Shape
	for _, inst := range layout.Instances {
		isEscape, err := db.IsEscapeCell(inst.Lib, inst.Cell)
		if err != nil {
			return nil, fmt.Errorf("netlist: instance %q: IsEscapeCell: %w", inst.Name, err)
		}
		if !isEscape {
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

// escapePathShapes returns an L-shaped PC path connecting pcShape to m1Pin,
// the M1 pin exactly as defined in cells.toml, and Contact cuts at the overlap
// of the horizontal PC bar and m1Pin.
//
// The L-shape is two PC rectangles:
//   - vertPC: original PC X width, Y extended to span both pcShape and m1Pin
//   - horizPC: at m1Pin's Y level, X spanning from min(pcX0,pinX0) to max(pcX1,pinX1)
//
// M1 is always exactly m1Pin — never widened.
func escapePathShapes(pcShape, m1Pin common.Shape, contactVC common.ViaConfig) []common.Shape {
	pcX0, pcY0 := pcShape.LowerLeft.X, pcShape.LowerLeft.Y
	pcX1, pcY1 := pcShape.UpperRight.X, pcShape.UpperRight.Y
	pinX0, pinY0 := m1Pin.LowerLeft.X, m1Pin.LowerLeft.Y
	pinX1, pinY1 := m1Pin.UpperRight.X, m1Pin.UpperRight.Y

	vertPC := common.Shape{
		Layer:      common.PC,
		LowerLeft:  common.Point{X: pcX0, Y: min(pcY0, pinY0)},
		UpperRight: common.Point{X: pcX1, Y: max(pcY1, pinY1)},
	}
	horizPC := common.Shape{
		Layer:      common.PC,
		LowerLeft:  common.Point{X: min(pcX0, pinX0), Y: pinY0},
		UpperRight: common.Point{X: max(pcX1, pinX1), Y: pinY1},
	}

	out := []common.Shape{vertPC, horizPC, m1Pin}
	return common.PlaceViaCuts(out, horizPC, m1Pin, contactVC, common.Contact, 0)
}
