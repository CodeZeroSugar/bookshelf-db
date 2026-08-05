package match

import (
	"regexp"
	"strings"
)

var nonAlnum = regexp.MustCompile(`[^a-z0-9 ]`)

// Normalize lowers a title, strips punctuation, and collapses whitespace so
// that "The Old Truck!", "the old truck", and "The    Old Truck" all match.
func Normalize(title string) string {
	s := strings.ToLower(title)
	s = nonAlnum.ReplaceAllString(s, " ")
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}
