// Package main provides a CLI helper for managing test secrets in the system keyring.
//
// Usage:
//
//	go run ./cmd/testsetup store <service> <key>    # reads value from stdin
//	go run ./cmd/testsetup get <service> <key>      # prints value to stdout
//	go run ./cmd/testsetup delete <service> <key>   # removes from keyring
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/zalando/go-keyring"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Fprintf(os.Stderr, "Usage: testsetup <store|get|delete> <service> <key>\n")
		os.Exit(1)
	}

	action := os.Args[1]
	service := os.Args[2]
	key := os.Args[3]

	switch action {
	case "store":
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading stdin: %v\n", err)
			os.Exit(1)
		}

		if err := keyring.Set(service, key, string(data)); err != nil {
			fmt.Fprintf(os.Stderr, "error storing in keyring: %v\n", err)
			os.Exit(1)
		}

		fmt.Fprintf(os.Stderr, "stored %d bytes in keyring (service=%q, key=%q)\n", len(data), service, key)

	case "get":
		val, err := keyring.Get(service, key)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading from keyring: %v\n", err)
			os.Exit(1)
		}

		fmt.Print(val)

	case "delete":
		if err := keyring.Delete(service, key); err != nil {
			fmt.Fprintf(os.Stderr, "error deleting from keyring: %v\n", err)
			os.Exit(1)
		}

		fmt.Fprintf(os.Stderr, "deleted from keyring (service=%q, key=%q)\n", service, key)

	default:
		fmt.Fprintf(os.Stderr, "unknown action: %s (use store, get, or delete)\n", action)
		os.Exit(1)
	}
}
