package indexcsv

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"os"
	"testing"
	"time"
)

//type Document = document.Document

func TestWriteDocumentsToCsv(t *testing.T) {

	// The index and the MD5 checksum must match for the test to work
	docsMap := map[string]Document{
		"abc": {Title: "Doc-1", Md5: "abc"},
		"def": {Title: "Doc-2", Md5: "def"},
	}

	tempFilename := fmt.Sprintf("mytempfile_%s.txt", time.Now().Format("20060102_150405"))

	// Call the function to test
	err := WriteDocumentsToCsv(docsMap, tempFilename)
	if err != nil {
		t.Fatalf("Unable to write docs to %s\n", tempFilename)
	}

	// The result should be a CSV with a header and two entries with correct Titles.
	// Parse as a CSV but process manually.
	content, err := os.ReadFile(tempFilename)
	if err != nil {
	}

	reader := csv.NewReader(bytes.NewReader(content))

	// Read all the records from the CSV
	csvRecords, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("FATAL: CSV record reading error for %s: %v", tempFilename, err)
	}

	// Verify each record against the appropriate Document.
	// It is assumed that the map key is the MD5 checksum.
	for idx, record := range csvRecords {
		if idx == 0 {
			// First record must be the header
			if (record[0] != "Record") || (record[1] != "Title") {
				t.Fatalf("FATAL: CSV header incorrect for %s, %v", tempFilename, record)
			}
		} else {
			key := record[6]
			d, present := docsMap[key]
			if !present {
				t.Fatalf("FATAL: CSV record %d from %s not found in docsMap %v", idx, tempFilename, docsMap)
			}
			title := record[1]
			if (d.Title != title) || (d.Md5 != key) {
				t.Fatalf("FATAL: CSV record %d from %s (%v) does not match docsMap %v", idx, record, tempFilename, d)
			}
		}
	}

	err = os.Remove(tempFilename)
	if err != nil {
		t.Fatalf("Error removing file %s: %v", tempFilename, err)
		return
	}
}
