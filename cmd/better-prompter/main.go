package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/better-prompter/better-prompter/internal/mcp"
	"github.com/better-prompter/better-prompter/internal/prompter"
)

const version = "1.0.0"

func main() {
	// Single-shot CLI mode: read prompt from stdin, write XML to stdout.
	// Usage: echo "my prompt" | better-prompter --enhance
	if len(os.Args) == 2 && os.Args[1] == "--enhance" {
		raw, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read error: %v\n", err)
			os.Exit(1)
		}
		prompt := strings.TrimSpace(string(raw))
		if prompt == "" {
			os.Exit(0)
		}
		xml, err := prompter.New().Enhance(context.Background(), prompt, "", "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "enhance error: %v\n", err)
			os.Exit(1)
		}
		fmt.Print(xml)
		return
	}

	// Default: MCP stdio server mode.
	// All logs go to stderr so they don't corrupt the MCP stdio stream.
	log.SetOutput(os.Stderr)
	log.SetFlags(0)

	srv := mcp.NewServer(mcp.Config{
		Name:     "better-prompter",
		Version:  version,
		Enhancer: prompter.New(),
	})

	log.Printf("better-prompter v%s ready", version)

	if err := srv.Run(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
