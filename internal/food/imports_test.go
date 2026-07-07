package food

import "testing"

func TestImportReceiptToPantryParsesGroceryLines(t *testing.T) {
	pantry, result := ImportReceiptToPantry("2 leche entera 1,80\nArroz redondo 1 kg 2,10\nTOTAL 3,90\n", Pantry{}, PantryImportOptions{})
	if len(result.Applied) != 2 {
		t.Fatalf("applied len = %d, want 2: %+v warnings=%+v", len(result.Applied), result.Applied, result.Warnings)
	}
	if len(pantry.Items) != 2 {
		t.Fatalf("pantry items = %+v", pantry.Items)
	}
	if pantry.Items[0].Name == "" || pantry.Items[0].Quantity <= 0 {
		t.Fatalf("bad pantry item: %+v", pantry.Items[0])
	}
	if result.Type != "receipt_imported" {
		t.Fatalf("type = %q", result.Type)
	}
}
