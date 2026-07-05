package basket

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

type Line struct {
	LineNo  int    `json:"line_no"`
	Ref     string `json:"ref"`
	Qty     string `json:"qty"`
	Comment string `json:"comment,omitempty"`
}

func Parse(r io.Reader) ([]Line, error) {
	scanner := bufio.NewScanner(r)
	var lines []Line
	for lineNo := 1; scanner.Scan(); lineNo++ {
		raw := scanner.Text()
		body, comment := splitComment(raw)
		body = strings.TrimSpace(body)
		if body == "" {
			continue
		}
		fields := strings.Fields(body)
		if len(fields) < 2 {
			return nil, fmt.Errorf("line %d: expected '<product_id_or_sku> <qty>'", lineNo)
		}
		lines = append(lines, Line{
			LineNo:  lineNo,
			Ref:     fields[0],
			Qty:     fields[1],
			Comment: strings.TrimSpace(comment),
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

func splitComment(s string) (body, comment string) {
	idx := strings.IndexByte(s, '#')
	if idx == -1 {
		return s, ""
	}
	return s[:idx], s[idx+1:]
}
