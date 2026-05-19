package common

import "fmt"

type Orient int

const (
	R0 Orient = iota
	MX
	MY
	R180
	R90
	R270
	MXR90
	MYR90
)

func (o Orient) MarshalText() ([]byte, error) {
	switch o {
	case R0:
		return []byte("R0"), nil
	case MX:
		return []byte("MX"), nil
	case MY:
		return []byte("MY"), nil
	case R180:
		return []byte("R180"), nil
	case R90:
		return []byte("R90"), nil
	case R270:
		return []byte("R270"), nil
	case MXR90:
		return []byte("MXR90"), nil
	case MYR90:
		return []byte("MYR90"), nil
	}
	return nil, fmt.Errorf("unknown orient: %d", o)
}

func (o *Orient) UnmarshalText(b []byte) error {
	switch string(b) {
	case "R0":
		*o = R0
	case "MX":
		*o = MX
	case "MY":
		*o = MY
	case "R180":
		*o = R180
	case "R90":
		*o = R90
	case "R270":
		*o = R270
	case "MXR90":
		*o = MXR90
	case "MYR90":
		*o = MYR90
	default:
		return fmt.Errorf("unknown orient: %q", string(b))
	}
	return nil
}

// SchematicInstance is an instance as it appears in the schematic,
// with coordinates in schematic units (float).
type SchematicInstance struct {
	Name   string  `json:"name"`
	Lib    string  `json:"lib"`
	Cell   string  `json:"cell"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Orient Orient  `json:"orient"`
}

// Instance is a placed layout instance, with coordinates in nm (int).
type Instance struct {
	Name   string `json:"name"`
	Lib    string `json:"lib"`
	Cell   string `json:"cell"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Orient Orient `json:"orient"`
}
