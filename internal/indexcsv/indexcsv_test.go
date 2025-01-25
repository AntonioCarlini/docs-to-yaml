package indexcsv

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"os"
	"reflect"
	"strings"
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

	tempFilename := HelperCreateTempFileName(0)

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
			if !reflect.DeepEqual(record, CsvHeadings()) {
				t.Fatalf("FATAL: CSV header incorrect for %s:\n      found: %v\n   expected: %v", tempFilename, record, CsvHeadings())
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

func TestReadDocumentsFromCsv(t *testing.T) {
	// The index and the MD5 checksum must match for the test to work
	docsMap := map[string]Document{
		"abc": {Title: "Doc-1", Md5: "abc", Filepath: "doc-1.pdf", Format: "PDF"},
		"def": {Title: "Doc-2", Md5: "def", Filepath: "doc-2.txt", Format: "TXT"},
	}

	// Write out a CSV file that matches the data in docsMap
	// Create the CSV file
	tempFilename := HelperCreateTempFileName(0)
	csvFile, err := os.Create(tempFilename)

	if err != nil {
		t.Fatalf("FATAL: Cannot create CSV file for %s: %v", tempFilename, err)
	}
	defer csvFile.Close()

	// Start with the header
	csvFile.WriteString(fmt.Sprintf("%s\n", strings.Join(CsvHeadings(), ",")))
	// For each Document, build a
	for _, doc := range docsMap {
		// Note: this assumes that doc is simple enough that no special character handling will be needed.
		csvFile.WriteString(fmt.Sprintf("%s\n", strings.Join(ConvertDocumentToCsv(doc), ",")))
	}
	csvFile.Close()

	// Now read back the CSV file and verify the same data comes back
	loadedDocsMap, err := ReadDocumentsFromCsv(tempFilename)
	if err != nil {
		t.Fatalf("FATAL: Cannot load manually created CSV file %s: %v", tempFilename, err)
	}

	if len(docsMap) != len(loadedDocsMap) {
		t.Fatalf("FATAL: started with %d docs but loaded %d from CSV", len(docsMap), len(loadedDocsMap))
	}

	// Walk through original docs map, ensuring that each key exists in the loaded documents and each original oc matches the loaded document
	for key, _ := range docsMap {
		if _, present := loadedDocsMap[key]; !present {
			t.Fatalf("FATAL: original key %s not present in map of loaded docs", key)
		}
		if docsMap[key] != loadedDocsMap[key] {
			t.Fatalf("FATAL: with key %s original doc %v does not match loaded doc %v", key, docsMap[key], loadedDocsMap[key])
		}
	}

	// Now perform the same checks but this time starting with loaded docs and checking against the originals
	for key, _ := range loadedDocsMap {
		if _, present := docsMap[key]; !present {
			t.Fatalf("FATAL: loaded key %s not present in map of original docs", key)
		}
		if loadedDocsMap[key] != docsMap[key] {
			t.Fatalf("FATAL: with key %s loaded doc %v does not match original doc %v", key, loadedDocsMap[key], docsMap[key])
		}
	}

	// Delete the CSV file that was created
	err = os.Remove(tempFilename)
	if err != nil {
		t.Fatalf("FATAL: cannot delete CSV file %s", tempFilename)
	}
}

func HelperCreateTempFileName(index int) string {
	return fmt.Sprintf("mytempfile_%s_%d.txt", time.Now().Format("20060102_150405"), index)
}
