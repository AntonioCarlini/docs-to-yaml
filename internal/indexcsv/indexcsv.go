package indexcsv

import (
	"docs-to-yaml/internal/document"
	"encoding/csv"
	"fmt"
	"log"
	"os"
)

type Document = document.Document

// The Document struct is how per-electronic-document data is represented in YAML
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

func WriteDocumentsToCsv(documents map[string]Document, filename string) error {

	csvFile, err := os.Create(filename)

	if err != nil {
		log.Fatalf("CSV file open failed for %s, %v\n", filename, err)
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
