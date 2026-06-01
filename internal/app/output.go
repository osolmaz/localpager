package app

import (
	"fmt"
	"io"
	"log"
)

func Printf(out io.Writer, format string, args ...any) {
	if _, err := fmt.Fprintf(out, format, args...); err != nil {
		log.Fatal(err)
	}
}

func Println(out io.Writer, value string) {
	if _, err := fmt.Fprintln(out, value); err != nil {
		log.Fatal(err)
	}
}
