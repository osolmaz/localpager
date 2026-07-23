package main

import (
	"io"
	"os"
	"reflect"
	"testing"

	"github.com/osolmaz/reposhell/reposhellcli"
)

func TestMainReturnsWithoutExitForSuccessfulCommand(t *testing.T) {
	restore := isolateMainProcess("status", "--config", "test.json")
	defer restore()

	runCLI = func(args []string, _ io.Reader, _, _ io.Writer, _ reposhellcli.Options) int {
		want := []string{"status", "--config", "test.json"}
		if !reflect.DeepEqual(args, want) {
			t.Fatalf("runCLI args = %q, want %q", args, want)
		}
		return 0
	}
	exitProcess = func(code int) {
		t.Fatalf("exitProcess(%d) called for successful command", code)
	}
	main()
}

func TestMainExitsWithCLIErrorCode(t *testing.T) {
	restore := isolateMainProcess("invalid")
	defer restore()

	runCLI = func([]string, io.Reader, io.Writer, io.Writer, reposhellcli.Options) int {
		return 2
	}
	exitCode := 0
	exitProcess = func(code int) {
		exitCode = code
	}
	main()
	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}
}

func isolateMainProcess(args ...string) func() {
	originalArgs := os.Args
	originalExit := exitProcess
	originalRun := runCLI
	os.Args = append([]string{"reposhell"}, args...)
	return func() {
		os.Args = originalArgs
		exitProcess = originalExit
		runCLI = originalRun
	}
}
