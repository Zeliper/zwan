//go:build !windows

package main

import "os"

// systemLanguage reads the usual environment variables. The desktop app targets
// Windows; this keeps the package building elsewhere.
func systemLanguage() string {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return ""
}
