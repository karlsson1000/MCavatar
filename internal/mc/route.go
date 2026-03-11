package mc

import (
	"strconv"
	"strings"
)

func ParseRoute(path, prefix string) (identifier string, size int) {
	path = strings.TrimPrefix(path, prefix)
	path = strings.TrimSuffix(path, ".png")
	path = strings.Trim(path, "/")

	parts := strings.SplitN(path, "/", 2)
	identifier = parts[0]
	size = DefaultSize

	if len(parts) == 2 {
		if n, err := strconv.Atoi(parts[1]); err == nil {
			size = ClampSize(n)
		}
	}
	return
}
