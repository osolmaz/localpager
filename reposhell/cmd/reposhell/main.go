package main

import (
	"os"

	"github.com/osolmaz/reposhell/reposhellcli"
)

func main() {
	code := reposhellcli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, reposhellcli.StandaloneOptions())
	if code != 0 {
		os.Exit(code)
	}
}
