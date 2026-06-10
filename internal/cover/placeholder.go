package cover

import _ "embed"

const PlaceholderMIME = "image/svg+xml"

//go:embed placeholder.svg
var placeholderSVG []byte

func Placeholder() []byte {
	return placeholderSVG
}
