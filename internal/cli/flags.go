package cli

import (
	"errors"
	"flag"
	"io"
	"strings"
)

var errHelpRequested = errors.New("help requested")

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
	if err := fs.Parse(reorderArgs(fs, args, boolFlags)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return errHelpRequested
		}
		return err
	}
	return nil
}

func reorderArgs(fs *flag.FlagSet, args []string, boolFlags map[string]bool) []string {
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
			if !hasValue && !isBoolFlag(fs, name, boolFlags) && i+1 < len(args) {
				flagsPart = append(flagsPart, args[i+1])
				i++
			}
			continue
		}
		positional = append(positional, arg)
	}
	return append(flagsPart, positional...)
}

func isBoolFlag(fs *flag.FlagSet, name string, boolFlags map[string]bool) bool {
	if boolFlags != nil && boolFlags[name] {
		return true
	}
	if fs == nil {
		return false
	}
	f := fs.Lookup(name)
	if f == nil {
		return false
	}
	if v, ok := f.Value.(interface{ IsBoolFlag() bool }); ok {
		return v.IsBoolFlag()
	}
	return false
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
