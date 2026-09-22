package input

import "math"

func relative(dx, dy float64) (int32, int32) {
	if math.IsNaN(dx) || math.IsInf(dx, 0) || math.IsNaN(dy) || math.IsInf(dy, 0) {
		return 0, 0
	}
	dx = math.Max(math.Min(dx, math.MaxInt32), math.MinInt32)
	dy = math.Max(math.Min(dy, math.MaxInt32), math.MinInt32)
	rx := int32(math.Round(dx))
	ry := int32(math.Round(dy))
	if rx == 0 && ry == 0 && (dx != 0 || dy != 0) {
		if math.Abs(dx) >= math.Abs(dy) {
			if dx > 0 {
				rx = 1
			} else {
				rx = -1
			}
		} else if dy > 0 {
			ry = 1
		} else {
			ry = -1
		}
	}
	return rx, ry
}
