package cli

import (
	"fmt"
	"io"
)

var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

func runVersion(stdout io.Writer) error {
	fmt.Fprintf(stdout, "carrito %s", Version)
	if Commit != "" && Commit != "unknown" {
		fmt.Fprintf(stdout, " (%s)", Commit)
	}
	if Date != "" && Date != "unknown" {
		fmt.Fprintf(stdout, " built %s", Date)
	}
	fmt.Fprintln(stdout)
	return nil
}
