package food

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func WritePDFFromJSONFile(inputPath, outputPath string) error {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return err
	}
	var probe map[string]any
	if err := json.Unmarshal(data, &probe); err != nil {
		return err
	}
	var title string
	var lines []string
	switch {
	case probe["selected_products"] != nil:
		var shop ShopResult
		if err := json.Unmarshal(data, &shop); err != nil {
			return err
		}
		title, lines = pdfLinesForShop(shop)
	case probe["recipes"] != nil:
		var collection struct {
			MealPlanID string   `json:"mealplan_id"`
			Recipes    []Recipe `json:"recipes"`
		}
		if err := json.Unmarshal(data, &collection); err != nil {
			return err
		}
		title, lines = pdfLinesForRecipes(collection.MealPlanID, collection.Recipes)
	case probe["days"] != nil:
		var plan MealPlan
		if err := json.Unmarshal(data, &plan); err != nil {
			return err
		}
		title, lines = pdfLinesForMealPlan(plan)
	case probe["ingredients"] != nil && probe["steps"] != nil:
		var recipe Recipe
		if err := json.Unmarshal(data, &recipe); err != nil {
			return err
		}
		title, lines = pdfLinesForRecipe(recipe)
	default:
		return fmt.Errorf("%s is not a supported food recipe, mealplan, or shop JSON file", inputPath)
	}
	return writeSimplePDF(outputPath, title, lines, filepath.Dir(inputPath))
}

func WriteSimplePDF(outputPath, title string, lines []string) error {
	return writeSimplePDF(outputPath, title, lines, "")
}

func writeSimplePDF(outputPath, title string, lines []string, imageBaseDir string) error {
	if strings.TrimSpace(outputPath) == "" {
		return fmt.Errorf("output path is required")
	}
	if title == "" {
		title = "Alcampo Food Plan"
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o700); err != nil && filepath.Dir(outputPath) != "." {
		return err
	}
	elements, images := pdfElements(title, lines, imageBaseDir)
	pages := layoutPDFPages(elements)
	if len(pages) == 0 {
		pages = []pdfPage{{Items: []pdfPageItem{{Text: title, FontSize: 18, X: 50, Y: 800}}}}
	}

	var objects [][]byte
	objects = append(objects, nil) // 1 catalog
	objects = append(objects, nil) // 2 pages
	objects = append(objects, []byte("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"))
	imageObjectNums := make(map[string]int, len(images))
	for _, img := range images {
		objNum := len(objects) + 1
		imageObjectNums[img.Name] = objNum
		objects = append(objects, img.Object())
	}
	pageObjectNums := make([]int, 0, len(pages))
	for _, page := range pages {
		contentObj := len(objects) + 1
		content := pdfContentStream(page)
		objects = append(objects, pdfStreamObject([]byte(content)))
		pageObj := len(objects) + 1
		pageObjectNums = append(pageObjectNums, pageObj)
		objects = append(objects, []byte(pdfPageObject(contentObj, imageObjectNums)))
	}
	var kids strings.Builder
	for _, num := range pageObjectNums {
		fmt.Fprintf(&kids, "%d 0 R ", num)
	}
	objects[0] = []byte("<< /Type /Catalog /Pages 2 0 R >>")
	objects[1] = []byte(fmt.Sprintf("<< /Type /Pages /Kids [ %s] /Count %d >>", kids.String(), len(pageObjectNums)))

	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects)+1)
	for i, obj := range objects {
		num := i + 1
		offsets[num] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n", num)
		buf.Write(obj)
		buf.WriteString("\nendobj\n")
	}
	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n", len(objects)+1)
	buf.WriteString("0000000000 65535 f \n")
	for i := 1; i <= len(objects); i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return os.WriteFile(outputPath, buf.Bytes(), 0o600)
}

func pdfLinesForMealPlan(plan MealPlan) (string, []string) {
	title := "Meal Plan"
	if plan.ID != "" {
		title = "Meal Plan " + plan.ID
	}
	lines := []string{
		fmt.Sprintf("People: %d", plan.People),
		"Selection policy: " + firstNonEmpty(plan.SelectionPolicy, "not set"),
		"Budget: " + firstNonEmpty(plan.BudgetEUR, "not set"),
		"",
	}
	for _, day := range plan.Days {
		lines = append(lines, fmt.Sprintf("Day %d", day.Day))
		for _, meal := range day.Meals {
			lines = append(lines, strings.ToUpper(meal.Type)+": "+meal.Recipe.Title)
			lines = append(lines, recipeSummaryLines(meal.Recipe)...)
		}
		lines = append(lines, "")
	}
	if len(plan.PantryUsage) > 0 {
		lines = append(lines, "Pantry and fridge used:")
		for _, use := range plan.PantryUsage {
			lines = append(lines, fmt.Sprintf("- %s: %.3g %s from %s", use.Ingredient, use.Quantity, use.Unit, use.PantryItem))
		}
		lines = append(lines, "")
	}
	if len(plan.RequiredPurchases) > 0 {
		lines = append(lines, "Shopping list:")
		for _, item := range plan.RequiredPurchases {
			lines = append(lines, fmt.Sprintf("- %s: %.3g %s", item.Name, item.Quantity, item.Unit))
		}
	}
	return title, lines
}

func pdfLinesForRecipe(recipe Recipe) (string, []string) {
	return recipe.Title, recipeSummaryLines(recipe)
}

func pdfLinesForRecipes(mealPlanID string, recipes []Recipe) (string, []string) {
	title := "Recipes"
	if mealPlanID != "" {
		title = "Recipes for " + mealPlanID
	}
	var lines []string
	for i, recipe := range recipes {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, recipe.Title)
		lines = append(lines, recipeSummaryLines(recipe)...)
	}
	if len(lines) == 0 {
		lines = append(lines, "No recipes found.")
	}
	return title, lines
}

func pdfLinesForShop(shop ShopResult) (string, []string) {
	lines := []string{
		"Selection policy: " + shop.Policy,
		"Estimated total: " + shop.EstimatedTotal.Amount + " " + firstNonEmpty(shop.EstimatedTotal.Currency, "EUR"),
		"",
		"Selected products:",
	}
	for _, selected := range shop.SelectedProducts {
		if selected.Error != "" {
			lines = append(lines, fmt.Sprintf("- %s: ERROR %s", selected.Ingredient.Name, selected.Error))
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s -> %s (%s)", selected.Ingredient.Name, selected.Product.Name, selected.Product.Price.Amount))
		if selected.Product.ImageURL != "" {
			lines = append(lines, "  Product image: "+selected.Product.ImageURL)
		}
		if selected.SelectionReason != "" {
			lines = append(lines, "  Reason: "+selected.SelectionReason)
		}
	}
	if len(shop.BasketLines) > 0 {
		lines = append(lines, "", "Basket lines:")
		lines = append(lines, shop.BasketLines...)
	}
	return "Alcampo Shopping Plan", lines
}

func recipeSummaryLines(recipe Recipe) []string {
	lines := []string{
		fmt.Sprintf("Servings: %d", recipe.Servings),
		fmt.Sprintf("Time: prep %d min, cook %d min", recipe.PrepMinutes, recipe.CookMinutes),
	}
	if recipe.ImageURL != "" {
		lines = append(lines, "Cover image: "+recipe.ImageURL)
	}
	if len(recipe.Tags) > 0 {
		lines = append(lines, "Tags: "+strings.Join(recipe.Tags, ", "))
	}
	lines = append(lines, "Ingredients:")
	for _, ing := range recipe.Ingredients {
		lines = append(lines, fmt.Sprintf("- %s: %.3g %s", ing.Name, ing.Quantity, ing.Unit))
	}
	if len(recipe.Equipment) > 0 {
		lines = append(lines, "Equipment: "+strings.Join(recipe.Equipment, ", "))
	}
	lines = append(lines, "Steps:")
	for _, step := range recipe.Steps {
		lines = append(lines, fmt.Sprintf("%d. %s", step.Number, step.Text))
		if step.ImageURL != "" {
			lines = append(lines, "   Step image: "+step.ImageURL)
		}
	}
	if len(recipe.Substitutions) > 0 {
		lines = append(lines, "Substitutions:")
		for _, sub := range recipe.Substitutions {
			lines = append(lines, "- "+sub)
		}
	}
	return lines
}

type pdfElement struct {
	Text     string
	FontSize float64
	Image    *pdfImageObject
	Caption  string
}

type pdfPage struct {
	Items []pdfPageItem
}

type pdfPageItem struct {
	Text     string
	FontSize float64
	X        float64
	Y        float64
	Image    *pdfImageObject
	Width    float64
	Height   float64
}

type pdfImageObject struct {
	Name   string
	Width  int
	Height int
	Data   []byte
}

func pdfElements(title string, lines []string, imageBaseDir string) ([]pdfElement, []pdfImageObject) {
	elements := []pdfElement{
		{Text: title, FontSize: 18},
		{Text: "", FontSize: 12},
	}
	var images []pdfImageObject
	imageByRef := make(map[string]*pdfImageObject)
	for _, line := range lines {
		if label, ref, ok := pdfImageReference(line); ok {
			if img, exists := imageByRef[ref]; exists {
				elements = append(elements, pdfElement{Image: img, Caption: label})
				continue
			}
			img, err := loadPDFImage(ref, len(images)+1, imageBaseDir)
			if err == nil {
				images = append(images, *img)
				imageByRef[ref] = &images[len(images)-1]
				elements = append(elements, pdfElement{Image: &images[len(images)-1], Caption: label})
				continue
			}
		}
		for _, wrapped := range wrapPDFLine(line, 92) {
			elements = append(elements, pdfElement{Text: wrapped, FontSize: 12})
		}
	}
	return elements, images
}

func pdfImageReference(line string) (string, string, bool) {
	trimmed := strings.TrimSpace(line)
	for _, prefix := range []string{"Cover image:", "Product image:", "Step image:"} {
		if strings.HasPrefix(trimmed, prefix) {
			ref := strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
			if ref == "" {
				return "", "", false
			}
			return strings.TrimSuffix(prefix, ":"), ref, true
		}
	}
	return "", "", false
}

func loadPDFImage(ref string, number int, imageBaseDir string) (*pdfImageObject, error) {
	data, err := readPDFImageData(ref, imageBaseDir)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	img = resizePDFImage(img, 640)
	bounds := img.Bounds()
	var raw bytes.Buffer
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			raw.Write(pdfRGB(img.At(x, y)))
		}
	}
	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	if _, err := zw.Write(raw.Bytes()); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return &pdfImageObject{
		Name:   fmt.Sprintf("Im%d", number),
		Width:  bounds.Dx(),
		Height: bounds.Dy(),
		Data:   compressed.Bytes(),
	}, nil
}

func readPDFImageData(ref string, imageBaseDir string) ([]byte, error) {
	ref = strings.TrimSpace(ref)
	if strings.HasPrefix(ref, "data:image/") {
		comma := strings.IndexByte(ref, ',')
		if comma < 0 {
			return nil, fmt.Errorf("invalid data image")
		}
		meta := ref[:comma]
		payload := ref[comma+1:]
		if strings.Contains(meta, ";base64") {
			return base64.StdEncoding.DecodeString(payload)
		}
		decoded, err := url.QueryUnescape(payload)
		if err != nil {
			return nil, err
		}
		return []byte(decoded), nil
	}
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		req, err := http.NewRequest(http.MethodGet, ref, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "alcampo-food-agent/1.0")
		client := http.Client{Timeout: 8 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("image fetch failed with %s", resp.Status)
		}
		return readLimited(resp.Body, 5*1024*1024)
	}
	if imageBaseDir != "" && !filepath.IsAbs(ref) {
		ref = filepath.Join(imageBaseDir, ref)
	}
	return os.ReadFile(ref)
}

func readLimited(r io.Reader, limit int64) ([]byte, error) {
	var buf bytes.Buffer
	n, err := io.Copy(&buf, io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if n > limit {
		return nil, fmt.Errorf("image exceeds %d bytes", limit)
	}
	return buf.Bytes(), nil
}

func resizePDFImage(img image.Image, maxSide int) image.Image {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= maxSide && height <= maxSide {
		return img
	}
	scale := math.Min(float64(maxSide)/float64(width), float64(maxSide)/float64(height))
	newWidth := max(1, int(math.Round(float64(width)*scale)))
	newHeight := max(1, int(math.Round(float64(height)*scale)))
	dst := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	for y := 0; y < newHeight; y++ {
		srcY := bounds.Min.Y + int(float64(y)/scale)
		if srcY >= bounds.Max.Y {
			srcY = bounds.Max.Y - 1
		}
		for x := 0; x < newWidth; x++ {
			srcX := bounds.Min.X + int(float64(x)/scale)
			if srcX >= bounds.Max.X {
				srcX = bounds.Max.X - 1
			}
			dst.Set(x, y, img.At(srcX, srcY))
		}
	}
	return dst
}

func pdfRGB(c color.Color) []byte {
	r, g, b, a := c.RGBA()
	if a == 0 {
		return []byte{255, 255, 255}
	}
	if a < 0xffff {
		r += 0xffff - a
		g += 0xffff - a
		b += 0xffff - a
	}
	return []byte{byte(r >> 8), byte(g >> 8), byte(b >> 8)}
}

func layoutPDFPages(elements []pdfElement) []pdfPage {
	const (
		pageHeight = 842.0
		margin     = 50.0
		lineHeight = 16.0
		imageMaxW  = 235.0
		imageMaxH  = 150.0
	)
	var pages []pdfPage
	page := pdfPage{}
	y := pageHeight - margin
	flush := func() {
		if len(page.Items) > 0 {
			pages = append(pages, page)
		}
		page = pdfPage{}
		y = pageHeight - margin
	}
	ensure := func(height float64) {
		if y-height < margin {
			flush()
		}
	}
	for _, el := range elements {
		if el.Image != nil {
			width, height := fitPDFImage(el.Image, imageMaxW, imageMaxH)
			needed := height + 22
			ensure(needed)
			imageY := y - height
			page.Items = append(page.Items, pdfPageItem{Image: el.Image, X: margin, Y: imageY, Width: width, Height: height})
			y = imageY - 12
			if el.Caption != "" {
				page.Items = append(page.Items, pdfPageItem{Text: el.Caption, FontSize: 10, X: margin, Y: y})
				y -= 14
			}
			continue
		}
		if el.Text == "" {
			ensure(8)
			y -= 8
			continue
		}
		fontSize := el.FontSize
		if fontSize <= 0 {
			fontSize = 12
		}
		height := lineHeight
		if fontSize > 12 {
			height = 24
		}
		ensure(height)
		page.Items = append(page.Items, pdfPageItem{Text: el.Text, FontSize: fontSize, X: margin, Y: y})
		y -= height
	}
	flush()
	return pages
}

func fitPDFImage(img *pdfImageObject, maxWidth, maxHeight float64) (float64, float64) {
	if img == nil || img.Width <= 0 || img.Height <= 0 {
		return maxWidth, maxHeight
	}
	scale := math.Min(maxWidth/float64(img.Width), maxHeight/float64(img.Height))
	if scale <= 0 {
		scale = 1
	}
	return float64(img.Width) * scale, float64(img.Height) * scale
}

func (img pdfImageObject) Object() []byte {
	header := fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /FlateDecode /Length %d >>\nstream\n", img.Width, img.Height, len(img.Data))
	var buf bytes.Buffer
	buf.WriteString(header)
	buf.Write(img.Data)
	buf.WriteString("\nendstream")
	return buf.Bytes()
}

func pdfStreamObject(data []byte) []byte {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "<< /Length %d >>\nstream\n", len(data))
	buf.Write(data)
	buf.WriteString("\nendstream")
	return buf.Bytes()
}

func pdfPageObject(contentObj int, imageObjectNums map[string]int) string {
	var xobjects strings.Builder
	for name, objNum := range imageObjectNums {
		fmt.Fprintf(&xobjects, "/%s %d 0 R ", name, objNum)
	}
	resources := "<< /Font << /F1 3 0 R >>"
	if xobjects.Len() > 0 {
		resources += " /XObject << " + xobjects.String() + ">>"
	}
	resources += " >>"
	return fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources %s /Contents %d 0 R >>", resources, contentObj)
}

func wrapPDFLine(line string, width int) []string {
	line = strings.TrimRight(line, "\r\n")
	if len(line) <= width {
		return []string{line}
	}
	var out []string
	words := strings.Fields(line)
	if len(words) == 0 {
		return []string{""}
	}
	current := words[0]
	for _, word := range words[1:] {
		if len(current)+1+len(word) > width {
			out = append(out, current)
			current = word
			continue
		}
		current += " " + word
	}
	if current != "" {
		out = append(out, current)
	}
	return out
}

func pdfContentStream(page pdfPage) string {
	var b strings.Builder
	for _, item := range page.Items {
		if item.Image != nil {
			fmt.Fprintf(&b, "q\n%.2f 0 0 %.2f %.2f %.2f cm\n/%s Do\nQ\n", item.Width, item.Height, item.X, item.Y, item.Image.Name)
			continue
		}
		fontSize := item.FontSize
		if fontSize <= 0 {
			fontSize = 12
		}
		fmt.Fprintf(&b, "BT\n/F1 %.0f Tf\n%.2f %.2f Td\n(%s) Tj\nET\n", fontSize, item.X, item.Y, escapePDFText(item.Text))
	}
	return b.String()
}

func escapePDFText(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "(", `\(`)
	s = strings.ReplaceAll(s, ")", `\)`)
	return s
}
