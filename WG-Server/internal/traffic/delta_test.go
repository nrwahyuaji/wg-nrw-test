package traffic

import "testing"

// TestDelta: kenaikan biasa, reset penghitung, dan tanpa perubahan.
func TestDelta(t *testing.T) {
	cases := []struct {
		name           string
		previous, curr Counter
		want           Counter
	}{
		{"naik", Counter{100, 50}, Counter{160, 80}, Counter{60, 30}},
		{"reset setelah reboot", Counter{9000, 7000}, Counter{120, 40}, Counter{120, 40}},
		{"diam", Counter{10, 10}, Counter{10, 10}, Counter{0, 0}},
	}
	for _, c := range cases {
		if got := Delta(c.previous, c.curr); got != c.want {
			t.Errorf("%s: got %+v want %+v", c.name, got, c.want)
		}
	}
}
