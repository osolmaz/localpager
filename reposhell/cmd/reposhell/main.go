package main

import (
	"os"

	"github.com/osolmaz/reposhell/reposhellcli"
)

var (
	exitProcess = os.Exit
	runCLI      = reposhellcli.Run
)

func main() {
	code := runCLI(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, reposhellcli.StandaloneOptions())
	if code != 0 {
		exitProcess(code)
	}
}

// mutate4go-manifest-begin
// {"version":1,"tested_at":"2026-07-23T12:31:57+08:00","module_hash":"ab40fe3cf28ace5e0464d2087c35dbc7528b45bc916fcd02e6a28e507b413345","functions":[{"id":"func/main","name":"main","line":14,"end_line":19,"hash":"6bb84a35d650b3331c8fba767051d1bf35f1d079c8ecb0f75e9d871d54ef9cfb"}]}
// mutate4go-manifest-end
