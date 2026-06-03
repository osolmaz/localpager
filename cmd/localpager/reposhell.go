package main

import (
	"os"

	"github.com/osolmaz/localpager/internal/reposhellcli"
)

func runReposhell(args []string) {
	os.Exit(reposhellcli.Run(args, os.Stdin, os.Stdout, os.Stderr, reposhellcli.LocalpagerOptions()))
}
