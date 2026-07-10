package cli

import (
	"fmt"
	"io"

	"github.com/wachtermar/carrito/internal/alcampo"
	"github.com/wachtermar/carrito/internal/money"
)

func printCartSummary(w io.Writer, summary alcampo.CartSummary) {
	for _, item := range summary.Items {
		fmt.Fprintf(w, "%s\tqty=%s\t€%s\t%s\n", firstNonEmpty(item.SKU, item.ProductID, "-"), item.Quantity, item.LineTotal.Amount, item.Name)
	}
	fmt.Fprintf(w, "total\t€%s\titems=%d\n", summary.Total.Amount, summary.ItemCount)
}

func formatMoney(value money.Money) string {
	return money.Format(value.Cents, firstNonEmpty(value.Currency, "EUR"))
}
