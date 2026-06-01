package app

import (
	"flag"
	"strings"

	"github.com/osolmaz/localpager/internal/notifier"
)

type MultiFlag []string

func (values *MultiFlag) String() string {
	return strings.Join(*values, ",")
}

func (values *MultiFlag) Set(value string) error {
	*values = append(*values, value)
	return nil
}

func SeenFlags(fs *flag.FlagSet) map[string]bool {
	flags := map[string]bool{}
	fs.Visit(func(f *flag.Flag) {
		flags[f.Name] = true
	})
	return flags
}

func ClosePool(pool *notifier.Pool) {
	_ = pool.Close()
}
