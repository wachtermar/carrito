package cli

import (
	"flag"
	"io"
	"strings"
)

func newFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}

func marketFlag(fs *flag.FlagSet, usage string) *string {
	v := fs.String("store", "", usage)
	fs.StringVar(v, "market", "", usage)
	return v
}

func parseInterspersed(fs *flag.FlagSet, args []string, boolFlags map[string]bool) error {
	return fs.Parse(reorderArgs(args, boolFlags))
}

func reorderArgs(args []string, boolFlags map[string]bool) []string {
	var flagsPart []string
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if isFlagToken(arg) {
			flagsPart = append(flagsPart, arg)
			name, hasValue := flagName(arg)
			if !hasValue && !boolFlags[name] && i+1 < len(args) {
				flagsPart = append(flagsPart, args[i+1])
				i++
			}
			continue
		}
		positional = append(positional, arg)
	}
	return append(flagsPart, positional...)
}

func isFlagToken(s string) bool {
	return strings.HasPrefix(s, "-") && s != "-" && !strings.HasPrefix(s, "-.")
}

func flagName(s string) (name string, hasValue bool) {
	s = strings.TrimLeft(s, "-")
	if idx := strings.IndexByte(s, '='); idx >= 0 {
		return s[:idx], true
	}
	return s, false
}
