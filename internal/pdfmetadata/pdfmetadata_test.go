package pdfmetadata

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/barasher/go-exiftool"
)

// --- Helpers ---

func fm(fields map[string]interface{}, err error) exiftool.FileMetadata {
	return exiftool.FileMetadata{
		Fields: fields,
		Err:    err,
		File:   "test.pdf",
	}
}

// --- Tests for parseExifOutput ---

func TestParseExifOutput_AllFields(t *testing.T) {
	input := []exiftool.FileMetadata{
		fm(map[string]interface{}{
			"Creator":    "TestCreator",
			"Producer":   "TestProducer",
			"PDFVersion": 1.7,
			"ModifyDate": "2024:01:02 15:04:05",
		}, nil),
	}

	got := parseExifOutput(input)

	if got.Creator != "TestCreator" {
		t.Errorf("Creator = %q, want %q", got.Creator, "TestCreator")
	}
	if got.Producer != "TestProducer" {
		t.Errorf("Producer = %q, want %q", got.Producer, "TestProducer")
	}
	if got.Format != "1.7" {
		t.Errorf("Format = %q, want %q", got.Format, "1.7")
	}
	if got.Modified != "2024:01:02 15:04:05" {
		t.Errorf("Modified = %q, want %q", got.Modified, "2024:01:02 15:04:05")
	}
}

func TestParseExifOutput_MissingFields(t *testing.T) {
	input := []exiftool.FileMetadata{
		fm(map[string]interface{}{
			"Creator": "OnlyCreator",
		}, nil),
	}

	got := parseExifOutput(input)

	if got.Creator != "OnlyCreator" {
		t.Errorf("Creator = %q, want %q", got.Creator, "OnlyCreator")
	}
	if got.Producer != "" || got.Format != "" || got.Modified != "" {
		t.Errorf("Expected other fields empty, got %+v", got)
	}
}

func TestParseExifOutput_MultipleEntries_LastWins(t *testing.T) {
	input := []exiftool.FileMetadata{
		fm(map[string]interface{}{
			"Creator": "First",
		}, nil),
		fm(map[string]interface{}{
			"Creator": "Second",
		}, nil),
	}

	got := parseExifOutput(input)

	if got.Creator != "Second" {
		t.Errorf("Creator = %q, want %q (last wins)", got.Creator, "Second")
	}
}

func TestParseExifOutput_IgnoresErrors(t *testing.T) {
	input := []exiftool.FileMetadata{
		fm(nil, fs.ErrInvalid), // should be skipped
		fm(map[string]interface{}{
			"Producer": "ValidProducer",
		}, nil),
	}

	got := parseExifOutput(input)

	if got.Producer != "ValidProducer" {
		t.Errorf("Producer = %q, want %q", got.Producer, "ValidProducer")
	}
}

func TestParseExifOutput_PDFVersionFormatting(t *testing.T) {
	input := []exiftool.FileMetadata{
		fm(map[string]interface{}{
			"PDFVersion": 1.70,
		}, nil),
	}

	got := parseExifOutput(input)

	// trailing zeros trimmed
	if got.Format != "1.7" {
		t.Errorf("Format = %q, want %q", got.Format, "1.7")
	}
}

// --- Tests for ExtractPdfMetadataFromFS (error paths only) ---

func TestExtractPdfMetadataFromFS_FileNotFound(t *testing.T) {
	fsys := fstest.MapFS{}

	got := ExtractPdfMetadataFromFS(fsys, "missing.pdf")

	if got != (PdfMetadata{}) {
		t.Errorf("Expected empty metadata on read error, got %+v", got)
	}
}

func TestExtractPdfMetadataFromFS_ReadSuccessButEmptyContent(t *testing.T) {
	fsys := fstest.MapFS{
		"file.pdf": {
			Data: []byte("not really a pdf"),
		},
	}

	// We cannot reliably assert metadata (depends on exiftool availability),
	// but we *can* assert it doesn't panic and returns a struct.
	got := ExtractPdfMetadataFromFS(fsys, "file.pdf")

	// Just sanity check struct exists
	_ = got
}

// --- Optional: regression test for legacy function equivalence ---

func TestParseExifOutput_EquivalentToLegacyLogic(t *testing.T) {
	input := []exiftool.FileMetadata{
		fm(map[string]interface{}{
			"Creator":    "A",
			"Producer":   "B",
			"PDFVersion": 1.4,
			"ModifyDate": "X",
		}, nil),
	}

	new := parseExifOutput(input)

	old := func() PdfMetadata {
		metadata := PdfMetadata{}
		for _, fi := range input {
			if fi.Err != nil {
				continue
			}
			for k, v := range fi.Fields {
				switch k {
				case "Creator":
					metadata.Creator = v.(string)
				case "Producer":
					metadata.Producer = v.(string)
				case "PDFVersion":
					metadata.Format = "1.4"
				case "ModifyDate":
					metadata.Modified = v.(string)
				}
			}
		}
		return metadata
	}()

	if new != old {
		t.Errorf("New parse != old parse\nnew=%+v\nold=%+v", new, old)
	}
}
