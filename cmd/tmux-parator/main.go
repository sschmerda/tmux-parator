package main

import (
	"fmt"
	"os"

	"github.com/sschmerda/tmux-parator/internal/app"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "--version", "-v":
			fmt.Fprintf(os.Stdout, "tmux-parator %s (%s, %s)\n", version, commit, date)
			return
		}
	}

	if err := app.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
