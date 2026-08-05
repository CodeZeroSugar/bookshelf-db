package match

import "testing"

func TestNormalize(t *testing.T) {
	cases := []struct{ in, want string }{
		{"The Old Truck", "the old truck"},
		{"the old truck", "the old truck"},
		{"The Old Truck!", "the old truck"},
		{"The    Old   Truck", "the old truck"},
		{"What Can You Do with a Paleta?", "what can you do with a paleta"},
		{"Swirl by Swirl", "swirl by swirl"},
		{"Bee-bim Bop!", "bee bim bop"},
		{"Inch by Inch", "inch by inch"},
		{"We're Going on a Leaf Hunt", "we re going on a leaf hunt"},
		{"  ", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := Normalize(c.in); got != c.want {
			t.Errorf("Normalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
