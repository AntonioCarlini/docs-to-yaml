package types

// The Document struct is how per-electronic-document data is represented in YAML
type Document struct {
	Format      string // File format (PDF, TXT, etc.)
	Size        int64  // File size in bytes
	Md5         string // File MD5 checksum
	Title       string // Document title
	PubDate     string // The publication date
	PartNum     string // The manufacturer identifier or part number for the document
	PdfCreator  string // PDF data: "Creator"
	PdfProducer string // PDF data: "Producer"
	PdfVersion  string // PDF data: "Format", this will be, for example, "PDF-1.2"
	PdfModified string // PDF data: "Modified"
}
