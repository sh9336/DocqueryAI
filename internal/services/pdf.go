package services

import (
	"bytes"
	"fmt"

	"github.com/ledongthuc/pdf"
)

// ExtractTextFromPDF reads a PDF file and extracts all text from it, along
// with its page count (used to enforce a max-pages-per-document limit
// before spending any Cohere embedding calls on it).
func ExtractTextFromPDF(filepath string) (string, int, error) {
	f, r, err := pdf.Open(filepath)
	if err != nil {
		return "", 0, fmt.Errorf("failed to open PDF: %w", err)
	}
	defer f.Close()

	var buf bytes.Buffer
	b, err := r.GetPlainText()
	if err != nil {
		return "", 0, fmt.Errorf("failed to get plain text from PDF: %w", err)
	}
	buf.ReadFrom(b)

	return buf.String(), r.NumPage(), nil
}
