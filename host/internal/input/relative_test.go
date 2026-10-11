package input

import (
	"math"
	"testing"
)

func TestRelativeSnapsSubpixel(t *testing.T) {
	dx, dy := relative(0.4, 0)
	if dx != 1 || dy != 0 {
		t.Fatalf("got %d,%d", dx, dy)
	}
	dx, dy = relative(0, -0.2)
	if dx != 0 || dy != -1 {
		t.Fatalf("got %d,%d", dx, dy)
	}
	dx, dy = relative(12.6, -3.4)
	if dx != 13 || dy != -3 {
		t.Fatalf("got %d,%d", dx, dy)
	}
}

func TestRelativeDoesNotOverflow(t *testing.T) {
	dx, dy := relative(1e100, -1e100)
	if dx != math.MaxInt32 || dy != math.MinInt32 {
		t.Fatalf("overflow reversed direction: %d,%d", dx, dy)
	}
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if dx, dy := relative(value, 1); dx != 0 || dy != 0 {
			t.Fatalf("nonfinite input became motion: %d,%d", dx, dy)
		}
	}
}
