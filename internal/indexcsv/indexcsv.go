package indexcsv

import (
	"bytes"
	"docs-to-yaml/internal/document"
	"encoding/csv"
	"fmt"
	"log"
	"os"
)

type Document = document.Document

type IndexCsv struct {
	RecordType  string // CSV Record Type
	Title       int64  // Document Title
	Filepath    string // Relative file path of document in collection
	PublicUrl   string // Public repository hosting the document; not necessarily originator of the docuemnt
	PubDate     string // The publication date
	PartNum     string // The manufacturer identifier or part number for the document
	Md5Checksum string // Document MD5 checksum
	Options     string // A string specifying rarer options
}

// UNTESTED: WIP
func WriteDocumentsToCsv(documents map[string]Document, filePath string) error {

	csvFile, err := os.Create(filePath)

	if err != nil {
		log.Fatalf("CSV file open failed for %s, %v\n", filePath, err)
	}
	defer csvFile.Close()

	var csvDocs [][]string

	for _, doc := range documents {
		csvDocs = append(csvDocs, ConvertDocumentToCsv(doc))
	}

	csvWriter := csv.NewWriter(csvFile)
	defer csvWriter.Flush()

	header := []string{"Record", "Title", "File", "URL", "Date", "Part Number", "MD5 Checksum", "Options"}
	err = csvWriter.Write(header)
	if err != nil {
		fmt.Println("Error writing header to CSV:", err)
		return err
	}

	for _, rec := range csvDocs {
		err = csvWriter.Write(rec)
		if err != nil {
			fmt.Println("Error writing record to CSV:", err)
			return err
		}
	}

	return nil
}

// UNTESTED: WIP
func ReadDocumentsFromCsv(filePath string) (map[string]Document, error) {

	docs := make(map[string]Document)

	fatal_error_seen := false

	content, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Printf("CSV file open failed for %s, %v\n", filePath, err)
		return docs, nil
	}

	// Read all the records from the CSV
	reader := csv.NewReader(bytes.NewReader(content))
	csvRecords, err := reader.ReadAll()
	if err != nil {
		fmt.Printf("FATAL: CSV record reading error for %s: %v", filePath, err)
		return docs, err
	}

	// The first record must be the header
	// header := []string{"Record", "Title", "File", "URL", "Date", "Part Number", "MD5 Checksum", "Options"}

	// Every "Doc" record needs to be converted into a Document object
	for _, rec := range csvRecords {
		if rec[0] != "Doc" {
			continue
		}
		var newDocument Document

		newDocument.Title = rec[1]
		newDocument.Filepath = rec[2]
		newDocument.PublicUrl = rec[3]
		newDocument.PubDate = rec[4]
		newDocument.PartNum = rec[5]
		newDocument.Md5 = rec[6]
		// TODO parse rec[7], which contains options, which includes 'collection=XXXX'
		// newDocument.Collection = ??
		newDocument.Format, err = document.DetermineDocumentFormat(rec[2])
		if err != nil {
			fmt.Printf("FATAL: CSV record reading error for %s: %v", rec[2], err)
			fatal_error_seen = true
		}
		// newDocument.Size = filestats.Size()
		// newDocument.PdfCreator = pdfMetadata.Creator
		// newDocument.PdfProducer = pdfMetadata.Producer
		// newDocument.PdfVersion = pdfMetadata.Format
		// newDocument.PdfModified = pdfMetadata.Modified
		// newDocument.Collection = "local-archive"

		// TODO what to do if MD5 not present?
		key := document.BuildKeyFromDocument(newDocument)
		docs[key] = newDocument

	}

	if fatal_error_seen {
		log.Fatal("FATAL: Stopping because of above FATAL error(s)")
	}

	return docs, nil
}

// This table shows the fields in a CSV record and the Document members from which each CSV field is derived.
//
// | Field #  | Contents             | CSV field
// |----------|----------------------|----------------
// |       1  | _ Record type_       | "Doc" (fixed text)
// |       2  | _Document title_     | .Title
// |       3  | _Local file path_    | .Filepath
// |       4  | _Original URL_       | .PublicUrl
// |       5  | _Document date_      | .PubDate
// |       6  | _Part number_        | .PartNum
// |       7  | _Options_            |
//
// The CSV 'options' field contains the following sub-options:
//
//	collection='' taken from Document.Collection
func ConvertDocumentToCsv(doc Document) []string {
	options := fmt.Sprintf("'collection=%s'", doc.Collection)
	return []string{
		"Doc",
		doc.Title,
		doc.Filepath,
		doc.PublicUrl,
		doc.PubDate,
		doc.PartNum,
		doc.Md5,
		options,
	}
}
