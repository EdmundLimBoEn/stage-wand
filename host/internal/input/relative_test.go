package input

import "testing"

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
