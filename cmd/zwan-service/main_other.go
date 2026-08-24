//go:build !windows

package main

import "fmt"

func main() {
	fmt.Println("zwan-service is Windows-only")
}
