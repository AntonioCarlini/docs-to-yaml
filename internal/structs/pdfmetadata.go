package types

// The PdfMetdata struct is used to record a subset of metadata that can be extracted from a PDF file
type PdfMetadata struct {
	Creator  string
	Producer string
	Format   string
	Modified string
}
