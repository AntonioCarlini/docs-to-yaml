package main

import (
	"bufio"
	"bytes"
	"docs-to-yaml/internal/document"
	"docs-to-yaml/internal/persistentstore"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// ----------------------------------------------------------------------
// Helper functions for creating temporary files and test data
// ----------------------------------------------------------------------

func createTempFile(t *testing.T, content string) string {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "test-*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer tmpFile.Close()
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}
	return tmpFile.Name()
}

func createTestMD5Store(t *testing.T, entries map[string]string) *persistentstore.Store[string, string] {
	t.Helper()
	store := &persistentstore.Store[string, string]{Data: make(map[string]string)}
	for key, md5 := range entries {
		store.Data[key] = md5
	}
	return store
}

// ----------------------------------------------------------------------
// Tests for FindAcceptablePaths
// ----------------------------------------------------------------------

func TestFindAcceptablePaths(t *testing.T) {
	tests := []struct {
		name              string
		indexContent      string
		expectedPaths     []string
		expectedLinesRead int // not directly returned, but we can log; we'll just check paths
	}{
		{
			name: "valid prefixes and acceptable file types",
			indexContent: `2021-09-24 22:05:17 dec/documentation/hardware/example.pdf
2021-09-24 22:05:18 able/firmware/rom.bin
2021-09-24 22:05:19 dilog/manual.txt
2021-09-24 22:05:20 emulex/schema.jpg
2021-09-24 22:05:21 mentec/guide.html
2021-09-24 22:05:22 terak/disk.doc
2021-09-24 22:05:23 other/ignore.pdf
		`,
			expectedPaths: []string{
				"dec/documentation/hardware/example.pdf",
				"dilog/manual.txt",
				"mentec/guide.html",
				"terak/disk.doc",
			},
		},
		{
			name: "reject file types",
			indexContent: `2021-09-24 22:05:17 dec/file.bin
2021-09-24 22:05:17 dec/file.gz
2021-09-24 22:05:17 dec/file.hex
2021-09-24 22:05:17 dec/file.jpg
2021-09-24 22:05:17 dec/file.lbl
2021-09-24 22:05:17 dec/file.lst
2021-09-24 22:05:17 dec/file.mcr
2021-09-24 22:05:17 dec/file.p75
2021-09-24 22:05:17 dec/file.png
2021-09-24 22:05:17 dec/file.pt
2021-09-24 22:05:17 dec/file.tar
2021-09-24 22:05:17 dec/file.tif
2021-09-24 22:05:17 dec/file.tiff
2021-09-24 22:05:17 dec/file.zip
2021-09-24 22:05:17 dec/file.dat
2021-09-24 22:05:17 dec/file.sav
2021-09-24 22:05:17 dec/file.jp2
`,
			expectedPaths: []string{}, // none should be accepted
		},
		{
			name: "accept file types",
			indexContent: `2021-09-24 22:05:17 dec/file.html
2021-09-24 22:05:17 dec/file.pdf
2021-09-24 22:05:17 dec/file.txt
2021-09-24 22:05:17 dec/file.doc
2021-09-24 22:05:17 dec/file.ln03
`,
			expectedPaths: []string{
				"dec/file.html",
				"dec/file.pdf",
				"dec/file.txt",
				"dec/file.doc",
				"dec/file.ln03",
			},
		},
		{
			name: "unknown file type prints warning but is accepted",
			indexContent: `2021-09-24 22:05:17 dec/file.xyz
`,
			expectedPaths: []string{"dec/file.xyz"},
		},
		{
			name: "mixed prefixes and types",
			indexContent: `2021-09-24 22:05:17 dec/ok.pdf
2021-09-24 22:05:17 able/ok.pdf
2021-09-24 22:05:17 dilog/ok.pdf
2021-09-24 22:05:17 emulex/ok.pdf
2021-09-24 22:05:17 mentec/ok.pdf
2021-09-24 22:05:17 terak/ok.pdf
2021-09-24 22:05:17 dec/bad.jpg
2021-09-24 22:05:17 other/bad.pdf
`,
			expectedPaths: []string{
				"able/ok.pdf",
				"dec/ok.pdf",
				"dilog/ok.pdf",
				"emulex/ok.pdf",
				"mentec/ok.pdf",
				"terak/ok.pdf",
			},
		},
		{
			name: "lines with fewer than 3 fields should be ignored (but may still be scanned)",
			indexContent: `2021-09-24 22:05:17 dec/ok.pdf
bad line
2021-09-25 10:00:00 able/file.txt
`,
			expectedPaths: []string{
				"able/file.txt",
				"dec/ok.pdf",
			},
		},
		{
			name: "sorting",
			indexContent: `2021-09-24 22:05:17 dec/b.pdf
2021-09-24 22:05:17 dec/a.pdf
2021-09-24 22:05:17 dec/c.pdf
`,
			expectedPaths: []string{
				"dec/a.pdf",
				"dec/b.pdf",
				"dec/c.pdf",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpFile := createTempFile(t, tt.indexContent)
			defer os.Remove(tmpFile)

			paths := FindAcceptablePaths(tmpFile)
			if paths == nil {
				paths = []string{}
			}

			// Filter out nil or empty strings (should not happen) but just in case
			filtered := []string{}
			for _, p := range paths {
				if p != "" {
					filtered = append(filtered, p)
				}
			}

			// Sort expected (already sorted in test cases)
			sort.Strings(tt.expectedPaths)

			if !reflect.DeepEqual(filtered, tt.expectedPaths) {
				t.Errorf("FindAcceptablePaths() = %v, want %v", filtered, tt.expectedPaths)
			}
		})
	}
}

// ----------------------------------------------------------------------
// Tests for MakeDocumentsFromPaths
// ----------------------------------------------------------------------

func TestMakeDocumentsFromPaths(t *testing.T) {
	// Helper to create a simple document with expected fields
	makeDoc := func(path, format, partNum, title, pubDate, md5 string) Document {
		return Document{
			Md5:         md5,
			PubDate:     pubDate,
			PdfCreator:  "",
			PdfProducer: "",
			PdfVersion:  "",
			PdfModified: "",
			Collection:  "bitsavers",
			Size:        0,
			PublicUrl:   bitsavers_prefix + path,
			Format:      format,
			PartNum:     partNum,
			Title:       title,
		}
	}

	tests := []struct {
		name              string
		md5Entries        map[string]string // key: full URL, value: MD5
		documentPaths     []string
		expectedMapSize   int
		expectedDuplicate int                 // we can capture output, but just check final map size and keys?
		expectedDropped   int                 // droppedDocument count printed, but we can't capture easily; instead verify keys not present
		expectedDocuments map[string]Document // key as used in map
	}{
		{
			name:            "basic parsing - part number and title",
			md5Entries:      map[string]string{},
			documentPaths:   []string{"dec/part123_title.pdf"},
			expectedMapSize: 1,
			expectedDocuments: map[string]Document{
				"PART: part123": makeDoc("dec/part123_title.pdf", "PDF", "part123", "title", "", "PART: part123"),
			},
		},
		{
			name:            "no part number",
			documentPaths:   []string{"dec/simple-document.pdf"},
			expectedMapSize: 1,
			expectedDocuments: map[string]Document{
				"TITLE: simple-document": makeDoc("dec/simple-document.pdf", "PDF", "", "simple-document", "", "TITLE: simple-document"),
			},
		},
		{
			name:            "extract date from title",
			documentPaths:   []string{"dec/part123_title_Dec91.pdf"},
			expectedMapSize: 1,
			expectedDocuments: map[string]Document{
				"PART: part123": makeDoc("dec/part123_title_Dec91.pdf", "PDF", "part123", "title", "1991-12", "PART: part123"),
			},
		},
		{
			name:            "date not recognized because month not valid",
			documentPaths:   []string{"dec/part123_title_Xxx91.pdf"},
			expectedMapSize: 1,
			expectedDocuments: map[string]Document{
				"PART: part123": makeDoc("dec/part123_title_Xxx91.pdf", "PDF", "part123", "title_Xxx91", "", "PART: part123"),
			},
		},
		{
			name:            "format derived from extension",
			documentPaths:   []string{"dec/part123_title.txt"},
			expectedMapSize: 1,
			expectedDocuments: map[string]Document{
				"PART: part123": makeDoc("dec/part123_title.txt", "TXT", "part123", "title", "", "PART: part123"),
			},
		},
		{
			name: "MD5 lookup success",
			md5Entries: map[string]string{
				bitsavers_prefix + "dec/known.pdf": "abcdef1234567890",
			},
			documentPaths:   []string{"dec/known.pdf"},
			expectedMapSize: 1,
			expectedDocuments: map[string]Document{
				"abcdef1234567890": makeDoc("dec/known.pdf", "PDF", "", "known", "", "abcdef1234567890"),
			},
		},
		{
			name: "filter out excluded prefixes",
			documentPaths: []string{
				"dec/pdp11/microfiche/Diagnostic_Program_Listings/bad.pdf",
				"dec/vax/microfiche/vms-source-listings/bad.txt",
				"dec/good.pdf",
			},
			expectedMapSize: 1,
			expectedDocuments: map[string]Document{
				"TITLE: good": makeDoc("dec/good.pdf", "PDF", "", "good", "", "TITLE: good"),
			},
		},
		{
			name: "duplicate key handling (same MD5)",
			md5Entries: map[string]string{
				bitsavers_prefix + "dec/dup1.pdf": "sameMD5",
				bitsavers_prefix + "dec/dup2.pdf": "sameMD5",
			},
			documentPaths:   []string{"dec/dup1.pdf", "dec/dup2.pdf"},
			expectedMapSize: 1, // one entry because key is MD5 and duplicates collapse
			expectedDocuments: map[string]Document{
				"sameMD5": makeDoc("dec/dup1.pdf", "PDF", "", "dup1", "", "sameMD5"),
			},
		},
		{
			name: "duplicate key handling (same dummy key by part number)",
			documentPaths: []string{
				"dec/part123_title1.pdf",
				"dec/part123_title2.pdf",
			},
			expectedMapSize: 1, // same PartNum, dummy key "PART: part123"
			expectedDocuments: map[string]Document{
				"PART: part123": makeDoc("dec/part123_title1.pdf", "PDF", "part123", "title1", "", "PART: part123"),
			},
		},
		{
			name: "duplicate key handling (same dummy key by title when no part number)",
			documentPaths: []string{
				"dec/sametitle.pdf",
				"dec/sametitle.pdf", // same path, duplicate
			},
			expectedMapSize: 1,
			expectedDocuments: map[string]Document{
				"TITLE: sametitle": makeDoc("dec/sametitle.pdf", "PDF", "", "sametitle", "", "TITLE: sametitle"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock MD5 store
			md5Store := createTestMD5Store(t, tt.md5Entries)

			// Capture stdout to avoid cluttering test output, but we can also discard
			oldStdout := os.Stdout
			_, w, _ := os.Pipe()
			os.Stdout = w

			docsMap := MakeDocumentsFromPaths("", tt.documentPaths, md5Store, false)

			w.Close()
			os.Stdout = oldStdout

			if len(docsMap) != tt.expectedMapSize {
				t.Errorf("Map size = %d, want %d", len(docsMap), tt.expectedMapSize)
			}

			// Verify each expected document exists with correct fields
			for key, expectedDoc := range tt.expectedDocuments {
				actualDoc, ok := docsMap[key]
				if !ok {
					t.Errorf("Expected key %q not found in map", key)
					continue
				}
				// Compare relevant fields (ignore PdfCreator, Producer, etc.)
				if actualDoc.Md5 != expectedDoc.Md5 {
					t.Errorf("Md5 mismatch for key %q: got %q, want %q", key, actualDoc.Md5, expectedDoc.Md5)
				}
				if actualDoc.PubDate != expectedDoc.PubDate {
					t.Errorf("PubDate mismatch for key %q: got %q, want %q", key, actualDoc.PubDate, expectedDoc.PubDate)
				}
				if actualDoc.Collection != expectedDoc.Collection {
					t.Errorf("Collection mismatch: got %q, want %q", actualDoc.Collection, expectedDoc.Collection)
				}
				if actualDoc.PublicUrl != expectedDoc.PublicUrl {
					t.Errorf("PublicUrl mismatch: got %q, want %q", actualDoc.PublicUrl, expectedDoc.PublicUrl)
				}
				if actualDoc.Format != expectedDoc.Format {
					t.Errorf("Format mismatch: got %q, want %q", actualDoc.Format, expectedDoc.Format)
				}
				if actualDoc.PartNum != expectedDoc.PartNum {
					t.Errorf("PartNum mismatch: got %q, want %q", actualDoc.PartNum, expectedDoc.PartNum)
				}
				if actualDoc.Title != expectedDoc.Title {
					t.Errorf("Title mismatch: got %q, want %q", actualDoc.Title, expectedDoc.Title)
				}
			}
		})
	}
}

// ----------------------------------------------------------------------
// Integration test: full pipeline from index file and MD5 file to YAML
// ----------------------------------------------------------------------

func TestIntegration(t *testing.T) {
	// Prepare a temporary directory for output
	tmpDir, err := os.MkdirTemp("", "bitsavers-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	indexContent := `2021-09-24 22:05:17 dec/part123_title_Dec91.pdf
2021-09-24 22:05:18 able/rom.bin
2021-09-24 22:05:19 mentec/guide.html
2021-09-24 22:05:20 dec/pdp11/microfiche/Diagnostic_Program_Listings/bad.pdf
2021-09-24 22:05:21 dec/known_md5.pdf
2021-09-24 22:05:22 dec/dup1.pdf
2021-09-24 22:05:23 dec/dup2.pdf
`
	indexFile := createTempFile(t, indexContent)
	defer os.Remove(indexFile)

	md5Content := `abcdef1234567890  dec/known_md5.pdf
1111111111111111  dec/dup1.pdf
1111111111111111  dec/dup2.pdf
`
	md5File := createTempFile(t, md5Content)
	defer os.Remove(md5File)

	outputYAML := filepath.Join(tmpDir, "output.yaml")

	// Step 1: Find acceptable paths
	paths := FindAcceptablePaths(indexFile)
	expectedPaths := []string{
		"dec/dup1.pdf",
		"dec/dup2.pdf",
		"dec/known_md5.pdf",
		"dec/part123_title_Dec91.pdf",
		"dec/pdp11/microfiche/Diagnostic_Program_Listings/bad.pdf",
		"mentec/guide.html",
	}
	if !reflect.DeepEqual(paths, expectedPaths) {
		t.Errorf("FindAcceptablePaths returned %v, want %v", paths, expectedPaths)
	}

	// Step 2: Load MD5 store
	md5Store := &persistentstore.Store[string, string]{
		Data: make(map[string]string),
	}
	md5Data, err := os.ReadFile(md5File)
	if err != nil {
		t.Fatalf("failed to read md5 file: %v", err)
	}
	scanner := bufio.NewScanner(strings.NewReader(string(md5Data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			md5sum := fields[0]
			relPath := fields[1]
			fullURL := bitsavers_prefix + relPath
			md5Store.Data[fullURL] = md5sum
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("error scanning md5 data: %v", err)
	}

	// Step 3: Build documents map
	docsMap := MakeDocumentsFromPaths("", paths, md5Store, false)

	expectedDocCount := 4
	if len(docsMap) != expectedDocCount {
		t.Errorf("Documents map size = %d, want %d", len(docsMap), expectedDocCount)
	}

	// Step 4: Write YAML
	err = document.WriteDocumentsMapToOrderedYaml(docsMap, outputYAML)
	if err != nil {
		t.Fatalf("WriteDocumentsMapToOrderedYaml failed: %v", err)
	}

	// Step 5: Read back and verify content
	yamlContent, err := os.ReadFile(outputYAML)
	if err != nil {
		t.Fatalf("failed to read output YAML: %v", err)
	}
	expectedYAMLSubstrings := []string{
		"md5: 'PART: part123'", // Quoted because of the colon
		"title: title",
		"pubdate: 1991-12",
		"md5: abcdef1234567890",   // Remains unquoted
		`md5: "1111111111111111"`, // Quoted because it looks like a number
	}
	for _, substr := range expectedYAMLSubstrings {
		if !bytes.Contains(yamlContent, []byte(substr)) {
			t.Errorf("YAML output missing expected substring %q  (content=[%s])", substr, yamlContent)
		}
	}
}

// ----------------------------------------------------------------------
// Test for contains helper
// ----------------------------------------------------------------------

func TestContains(t *testing.T) {
	tests := []struct {
		slice     []string
		candidate string
		expected  bool
	}{
		{[]string{"a", "b", "c"}, "b", true},
		{[]string{"a", "b", "c"}, "d", false},
		{[]string{}, "a", false},
	}

	for _, tt := range tests {
		result := contains(tt.slice, tt.candidate)
		if result != tt.expected {
			t.Errorf("contains(%v, %q) = %v, want %v", tt.slice, tt.candidate, result, tt.expected)
		}
	}
}

// ----------------------------------------------------------------------
// Test for CreateBitsaversDocument
// ----------------------------------------------------------------------

func TestCreateBitsaversDocument(t *testing.T) {
	path := "dec/test.pdf"
	doc := CreateBitsaversDocument(path)
	if doc.Md5 != "" {
		t.Errorf("Md5 should be empty, got %q", doc.Md5)
	}
	if doc.PubDate != "" {
		t.Errorf("PubDate should be empty, got %q", doc.PubDate)
	}
	if doc.Collection != "bitsavers" {
		t.Errorf("Collection = %q, want bitsavers", doc.Collection)
	}
	expectedURL := bitsavers_prefix + path
	if doc.PublicUrl != expectedURL {
		t.Errorf("PublicUrl = %q, want %q", doc.PublicUrl, expectedURL)
	}
}
