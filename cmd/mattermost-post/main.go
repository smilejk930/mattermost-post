package main

import (
	"os"

	"github.com/smilejk930/mattermost-post/internal/app"
)

var version = "dev"

func main() {
	runner := app.New(version)
	os.Exit(runner.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
