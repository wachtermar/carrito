package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func openInput(path string) (io.Reader, func(), error) {
	if path == "-" {
		return os.Stdin, func() {}, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, func() {}, err
	}
	return f, func() { _ = f.Close() }, nil
}

func readInputBytes(path string) ([]byte, error) {
	r, closeFn, err := openInput(path)
	if err != nil {
		return nil, err
	}
	defer closeFn()
	return io.ReadAll(r)
}

func readClipboard() ([]byte, error) {
	var candidates [][]string
	switch runtime.GOOS {
	case "darwin":
		candidates = [][]string{{"pbpaste"}}
	case "windows":
		candidates = [][]string{{"powershell", "-NoProfile", "-Command", "Get-Clipboard"}}
	default:
		candidates = [][]string{
			{"wl-paste", "--no-newline"},
			{"xclip", "-selection", "clipboard", "-out"},
			{"xsel", "--clipboard", "--output"},
		}
	}
	var lastErr error
	for _, candidate := range candidates {
		out, err := exec.Command(candidate[0], candidate[1:]...).Output()
		if err == nil && len(strings.TrimSpace(string(out))) > 0 {
			return out, nil
		}
		if err != nil {
			lastErr = err
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("could not read clipboard: %w", lastErr)
	}
	return nil, errors.New("clipboard was empty")
}
