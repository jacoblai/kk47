package deny

import (
	"bytes"
)

func Injection(src []byte) bool {
	if bytes.ContainsAny(src, "$|&;") {
		return false
	}
	return true
}
