package netlist

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseOrient(t *testing.T) {
	assert.Equal(t, "R0", parseOrient(`"R0"`))
	assert.Equal(t, "MX", parseOrient(`"MX"`))
	assert.Equal(t, "MY", parseOrient(`"MY"`))
	assert.Equal(t, "R180", parseOrient(`"R180"`))
	assert.Equal(t, "R0", parseOrient("R0"))
}

func TestTransformPin(t *testing.T) {
	xL, xH, yL, yH := 100, 200, 300, 400

	t.Run("R0", func(t *testing.T) {
		xl, xh, yl, yh := transformPin(xL, xH, yL, yH, "R0")
		assert.Equal(t, 100, xl)
		assert.Equal(t, 200, xh)
		assert.Equal(t, 300, yl)
		assert.Equal(t, 400, yh)
	})
	t.Run("MX", func(t *testing.T) {
		xl, xh, yl, yh := transformPin(xL, xH, yL, yH, "MX")
		assert.Equal(t, 100, xl)
		assert.Equal(t, 200, xh)
		assert.Equal(t, -400, yl)
		assert.Equal(t, -300, yh)
	})
	t.Run("MY", func(t *testing.T) {
		xl, xh, yl, yh := transformPin(xL, xH, yL, yH, "MY")
		assert.Equal(t, -200, xl)
		assert.Equal(t, -100, xh)
		assert.Equal(t, 300, yl)
		assert.Equal(t, 400, yh)
	})
	t.Run("R180", func(t *testing.T) {
		xl, xh, yl, yh := transformPin(xL, xH, yL, yH, "R180")
		assert.Equal(t, -200, xl)
		assert.Equal(t, -100, xh)
		assert.Equal(t, -400, yl)
		assert.Equal(t, -300, yh)
	})
	t.Run("unknown acts as R0", func(t *testing.T) {
		xl, xh, yl, yh := transformPin(xL, xH, yL, yH, "XYZ")
		assert.Equal(t, xL, xl)
		assert.Equal(t, xH, xh)
		assert.Equal(t, yL, yl)
		assert.Equal(t, yH, yh)
	})
}
