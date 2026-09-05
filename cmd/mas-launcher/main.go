package main

import (
	"fmt"
	"os"

	"github.com/MuikaAI/mas-launcher/internal/core"
)

func main() {
	if err := core.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
