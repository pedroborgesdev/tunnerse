//go:build !windows

package main

import "context"

func runEntrypoint() error {
	return runServer(context.Background())
}
