//go:build !windows

package web

import "fmt"

func pickDirectory(title string, allowNew bool) (string, error) {
	return "", fmt.Errorf("folder browsing is only available on Windows")
}
