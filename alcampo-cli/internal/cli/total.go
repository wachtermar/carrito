package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"alcampo-cli/internal/alcampo"
	"alcampo-cli/internal/basket"
	"alcampo-cli/internal/money"
	"alcampo-cli/internal/output"
)

func runTotal(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("total", stderr)
	file := fs.String("file", "", "basket file, or - for stdin")
	fs.StringVar(file, "f", "", "basket file, or - for stdin")
	jsonOut := fs.Bool("json", false, "write JSON to stdout")
	maxEUR := fs.String("max", "", "optional spending guard in EUR")
	store := marketFlag(fs, "Alcampo region/store UUID to use for pricing")
	if err := parseInterspersed(fs, args, map[string]bool{"json": true}); err != nil {
		return err
	}
	if *file == "" {
		return errors.New("total requires -f <basket-file|->")
	}
	cfg, client, err := newClient(*store)
	if err != nil {
		return err
	}
	regionID, err := resolveMarket(cfg, *store)
	if err != nil {
		return err
	}
	client.RegionID = regionID
	r, closeFn, err := openInput(*file)
	if err != nil {
		return err
	}
	defer closeFn()
	lines, err := basket.Parse(r)
	if err != nil {
		return err
	}
	result := TotalResult{Complete: true, Currency: "EUR"}
	for _, line := range lines {
		item := TotalLine{LineNo: line.LineNo, Ref: line.Ref, Qty: line.Qty, Comment: line.Comment}
		p, err := client.Lookup(context.Background(), line.Ref)
		if err != nil {
			item.Error = err.Error()
			result.Complete = false
			result.Lines = append(result.Lines, item)
			continue
		}
		item.Product = &p
		if p.Price.Amount == "" {
			item.Error = "price unavailable"
			result.Complete = false
			result.Lines = append(result.Lines, item)
			continue
		}
		lineCents, err := money.MultiplyCentsByQuantity(p.Price.Cents, line.Qty)
		if err != nil {
			item.Error = err.Error()
			result.Complete = false
			result.Lines = append(result.Lines, item)
			continue
		}
		item.LineTotal = money.Money{Amount: money.FormatAmount(lineCents), Currency: "EUR", Cents: lineCents}
		result.TotalCents += lineCents
		result.Lines = append(result.Lines, item)
	}
	result.Total = money.Money{Amount: money.FormatAmount(result.TotalCents), Currency: "EUR", Cents: result.TotalCents}
	if err := enforceMax(result.TotalCents, *maxEUR, cfg); err != nil {
		return err
	}
	if *jsonOut {
		return output.JSON(stdout, result)
	}
	for _, line := range result.Lines {
		if line.Error != "" {
			fmt.Fprintf(stdout, "line %d\t%s x %s\tERROR\t%s\n", line.LineNo, line.Ref, line.Qty, line.Error)
			continue
		}
		fmt.Fprintf(stdout, "line %d\t%s x %s\t%s\t%s\n", line.LineNo, line.Product.SKU, line.Qty, money.Format(line.LineTotal.Cents, "EUR"), line.Product.Name)
	}
	fmt.Fprintf(stdout, "total\t%s\tcomplete=%t\n", money.Format(result.TotalCents, "EUR"), result.Complete)
	return nil
}

type TotalResult struct {
	Complete   bool        `json:"complete"`
	Currency   string      `json:"currency"`
	TotalCents int64       `json:"total_cents"`
	Total      money.Money `json:"total"`
	Lines      []TotalLine `json:"lines"`
}

type TotalLine struct {
	LineNo    int              `json:"line_no"`
	Ref       string           `json:"ref"`
	Qty       string           `json:"qty"`
	Comment   string           `json:"comment,omitempty"`
	Product   *alcampo.Product `json:"product,omitempty"`
	LineTotal money.Money      `json:"line_total,omitempty"`
	Error     string           `json:"error,omitempty"`
}
