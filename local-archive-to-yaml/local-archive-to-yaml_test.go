package main

import (
	"crypto/md5"
	"docs-to-yaml/internal/persistentstore"
	"encoding/hex"
	"io/fs"
	"testing"
	"testing/fstest"
)

// func TestParseIndirectFile(t *testing.T) {
// 	indirectFile, err := os.CreateTemp("", "docs-to-yaml-local-to-yaml*.txt")
// 	if err != nil {
// 		t.Fatalf("Cannot create temporary file")
// 	}
// 	fn := indirectFile.Name()
// 	fmt.Println("temp file = ", fn)
// 	indirectFile.Close()

// 	ok1_indirect := [][]string{{"/path/tree/file01.txt", "0001"}, {"/path/tree2/file02.txt", "0002"}, {"/path/tree3/file03.txt", "0003"}}
// 	err = CheckIndirectFileResponse(fn, ok1_indirect, false)
// 	if err != nil {
// 		t.Fatalf("Failed ParseIndirectFile(ok1_indirect) = %s", err)
// 	}

// 	ok2_indirect := [][]string{{"/path/tree/file01.txt", "0001", "/path/other/root"}, {"/path/tree2/file02.txt", "0002"}, {"/path/tree3/file03.txt", "0003"}}
// 	err = CheckIndirectFileResponse(fn, ok2_indirect, false)
// 	if err != nil {
// 		t.Fatalf("Failed ParseIndirectFile(ok2_indirect) = %s", err)
// 	}

// 	ok3_indirect := [][]string{{"/path/tree/file01.txt", "0001", "/path/other/root"}, {"\"/path/includes a space/file02.txt\"", "0002"}, {"/path/tree3/file03.txt", "0003"}}
// 	err = CheckIndirectFileResponse(fn, ok3_indirect, false)
// 	if err != nil {
// 		t.Fatalf("Failed ParseIndirectFile(ok3_indirect) = %s", err)
// 	}

// 	// Line 2 has only one value
// 	fail1_indirect := [][]string{{"/path/tree/file01.txt", "0001", "/path/other/root"}, {"/path/tree2/file02.txt"}, {"/path/tree3/file03.txt", "0003"}}
// 	err = CheckIndirectFileResponse(fn, fail1_indirect, true)
// 	if err != nil {
// 		t.Fatalf("Failed ParseIndirectFile(fail1_indirect) = %s", err)
// 	}

// 	// Clear up by removing the temporary file
// 	os.Remove(fn)
// }

// func CheckIndirectFileResponse(indirectFilename string, data [][]string, expectError bool) error {
// 	indirectFile, err := os.OpenFile(indirectFilename, os.O_WRONLY, 0644)
// 	if err != nil {
// 		return err
// 	}

// 	for _, v := range data {
// 		text := strings.Join(v, " ")
// 		indirectFile.WriteString(text + "\n")
// 	}
// 	indirectFile.Close()

// 	result, err := ParseIndirectFile(indirectFilename)
// 	if expectError && (err == nil) {
// 		return fmt.Errorf("Expected error but ParseIndirectFile() returned success")
// 	} else if !expectError && (err != nil) {
// 		return fmt.Errorf("Expected success but ParseIndirectFile() returned error: %s", err)
// 	}

// 	// If an error has been signalled, there's no point checking the data itself.
// 	// We also do not check the nature of the error: that there has been an error signalled is enough of a test.
// 	if err != nil {
// 		return nil
// 	}

// 	if len(result) != len(data) {
// 		return fmt.Errorf("incoming data has %d elements, but result has %d; err=%s; data in = %#v", len(data), len(result), err, data)
// 	} else {
// 		for k, v := range result {
// 			path := ""
// 			volume := ""
// 			root := ""
// 			switch len(data[k]) {
// 			case 0:
// 			case 1:
// 				path = data[k][0]
// 				root = filepath.Dir(path)
// 			case 2:
// 				path = data[k][0]
// 				volume = data[k][1]
// 				root = filepath.Dir(data[k][0])
// 			case 3:
// 				path = data[k][0]
// 				volume = data[k][1]
// 				root = data[k][2]
// 			}
// 			// If resulting path includes a leading and final double quote remove them.
// 			// In this case also remove a leading double quote from root, if one is present.
// 			if (path[0] == '"') && (path[len(path)-1] == '"') {
// 				path = path[1 : len(path)-1]
// 				if root[0] == '"' {
// 					root = root[1:]
// 				}
// 			}
// 			if (v.Path != path) || (v.Volume != volume) || (v.Root != root) {
// 				return fmt.Errorf("mismatched result at entry %d: {%s},{%s},{%s} != {%s},{%s},{%s}", k, v.Path, v.Volume, v.Root, path, volume, root)
// 			}
// 		}
// 	}
// 	return nil

// }

func TestTidyDocumentTitle(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// Test case 1: Trim whitespace
		{"  Hello World  ", "Hello World"}, // Leading and trailing spaces

		// Test case 2: Removing CRLF
		{"Title\r\nwith CRLF", "Titlewith CRLF"}, // CRLF characters should be removed

		// Test case 3: Collapsing multiple spaces into a single space
		{"Hello     World", "Hello World"}, // Multiple spaces should collapse

		// Test case 4: Handling <BR> tags
		{"Hello <BR> World", "Hello. World"},      // Single <BR> should be replaced with ". "
		{"Hello <BR><BR> World", "Hello. World"},  // Multiple <BR> should be replaced with ". "
		{"Hello <BR> <BR> World", "Hello. World"}, // Spaces around <BR> should be handled
		{"Hello World <BR>", "Hello World. "},     // <BR> at the end should be replaced

		// Test case 5: Combination of multiple rules
		{"  Hello <BR>  World  <BR><BR> !  ", "Hello. World. !"}, // Multiple issues: spaces, <BR>, etc.

		// Test case 6: Empty string
		{"", ""}, // Empty string should return empty string

		// Test case 7: Only <BR> tags, should replace all <BR> tags with ". "
		{"<BR><BR><BR>", ". "}, // All <BR> should be replaced with ". "

		// Test case 8: Special case of leading and trailing <BR> tags
		{"<BR>Hello World<BR>", ". Hello World. "}, // <BR> before and after should be replaced

		// Test case 9: String with no spaces or <BR> tags (no change expected)
		{"HelloWorld", "HelloWorld"}, // No spaces, no <BR> tags, should remain the same
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			result := TidyDocumentTitle(test.input)
			if result != test.expected {
				t.Errorf("For input '%s', expected '%s' but got '%s'", test.input, test.expected, result)
			}
		})
	}
}
func TestStripOptionalLeadingAndTrailingDoubleQuotes(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},                           // Empty string
		{"hello world", "hello world"},     // No quotes
		{"hellorld!", "hellorld!"},         // No quotes, but with extra Usagi Electric
		{"\"hello world\"", "hello world"}, // With quotes beginning and end
		{"\"\"", ""},                       // Quotes beginning and end but nothing in between
		{"\"\"\"", "\""},                   // Quotes beginning and end and another quote in between
		{"\"foo\"bar", "\"foo\"bar"},       // Quotes beginning and end and another quote in between along with other text
		{"\"a very long string that should have quotes removed\"", "a very long string that should have quotes removed"}, // Long string, with quotes to remove
		{"\"some \\\"quoted\\\" text\"", "some \\\"quoted\\\" text"},                                                     // String with escaped quotes (does not handle escape sequences)
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			result := StripOptionalLeadingAndTrailingDoubleQuotes(test.input)
			if result != test.expected {
				t.Errorf("For input '%s', expected '%s' but got '%s'", test.input, test.expected, result)
			}
		})
	}
}

func TestCalculateMd5Sum(t *testing.T) {
	// 1. Setup mock file system and expected values
	fileContent := []byte("example document content")
	hasher := md5.New()
	hasher.Write(fileContent)
	realMd5 := hex.EncodeToString(hasher.Sum(nil))

	mockFS := fstest.MapFS{
		"folder/doc.pdf": &fstest.MapFile{Data: fileContent},
	}

	t.Run("Cache Hit", func(t *testing.T) {
		// Pre-populate the store with a "fake" hash to prove the function
		// returns the cached value without reading the file system.
		fakeHash := "cached-hash-123"
		store := &persistentstore.Store[string, string]{
			Data: map[string]string{"vol1//folder/doc.pdf": fakeHash},
		}

		// CalculateMd5Sum should return the fakeHash immediately.
		result, err := CalculateMd5Sum(mockFS, "vol1//folder/doc.pdf", "folder/doc.pdf", store, false)
		if err != nil {
			t.Fatalf("Unexpected error on cache hit: %v", err)
		}
		if result != fakeHash {
			t.Errorf("Expected cached value %s, but got %s", fakeHash, result)
		}
	})

	t.Run("Cache Miss", func(t *testing.T) {
		// Use an empty store
		store := &persistentstore.Store[string, string]{
			Data: make(map[string]string),
		}

		// CalculateMd5Sum must read the FS, calculate the hash, and update the store.
		result, err := CalculateMd5Sum(mockFS, "vol1//folder/doc.pdf", "folder/doc.pdf", store, false)
		if err != nil {
			t.Fatalf("Unexpected error on cache miss: %v", err)
		}
		if result != realMd5 {
			t.Errorf("Expected calculated hash %s, but got %s", realMd5, result)
		}

		// Verify the store was actually updated
		cachedVal, found := store.Lookup("vol1//folder/doc.pdf")
		if !found || cachedVal != realMd5 {
			t.Errorf("Persistent store was not updated correctly after cache miss")
		}
	})

	t.Run("File Missing Error", func(t *testing.T) {
		store := &persistentstore.Store[string, string]{
			Data: make(map[string]string),
		}

		// Attempt to hash a file that does not exist in our mockFS
		_, err := CalculateMd5Sum(mockFS, "vol1//missing.txt", "missing.txt", store, false)
		if err == nil {
			t.Error("Expected an error when attempting to hash a non-existent file, but got nil")
		}
	})
}

func TestDetermineCategory(t *testing.T) {
	tests := []struct {
		name     string
		files    fstest.MapFS
		expected ArchiveCategory
	}{
		{
			name: "Regular Archive",
			// Needs index.htm to be valid
			files: fstest.MapFS{
				"index.htm": &fstest.MapFile{Data: []byte("<html></html>")},
			},
			expected: AC_Regular,
		},
		{
			name: "HTML Archive",
			// Needs INDEX.HTM and /HTML
			// Must NOT have index.htm (lowercase) or it triggers conflict
			files: fstest.MapFS{
				"INDEX.HTM": &fstest.MapFile{Data: []byte("<html></html>")},
				"HTML/":     &fstest.MapFile{Mode: fs.ModeDir},
			},
			expected: AC_HTML,
		},
		{
			name: "Metadata Archive",
			// Needs index.htm to pass the validity check
			files: fstest.MapFS{
				"index.htm": &fstest.MapFile{Data: []byte("<html></html>")},
				"metadata/": &fstest.MapFile{Mode: fs.ModeDir},
			},
			expected: AC_Metadata,
		},
		{
			name: "Custom Archive (CRC File)",
			// Needs index.htm to pass the validity check
			files: fstest.MapFS{
				"index.htm":    &fstest.MapFile{Data: []byte("<html></html>")},
				"DEC_0040.CRC": &fstest.MapFile{Data: []byte("some-crc-data")},
			},
			expected: AC_Custom,
		},
		{
			name: "Invalid (Conflict: HTML and Metadata)",
			// Triggers the "conflicting files" check in the INDEX.HTM block[cite: 3]
			files: fstest.MapFS{
				"INDEX.HTM": &fstest.MapFile{Data: []byte("<html></html>")},
				"HTML/":     &fstest.MapFile{Mode: fs.ModeDir},
				"metadata/": &fstest.MapFile{Mode: fs.ModeDir},
			},
			expected: AC_Undefined,
		},
		{
			name:     "Invalid (Empty FS - No index.htm)",
			files:    fstest.MapFS{},
			expected: AC_Undefined,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			category := DetermineCategory(tt.files, "test-volume")
			if category != tt.expected {
				t.Errorf("Expected category %v, but got %v", tt.expected, category)
			}
		})
	}
}

func TestResolvePathCaseInsensitive(t *testing.T) {
	mockFS := fstest.MapFS{
		"Folder/SubFolder/Document.pdf": &fstest.MapFile{Data: []byte("pdf data")},
		"images/PHOTO.JPG":              &fstest.MapFile{Data: []byte("image data")},
		// This filename is exactly 64 characters total (60 chars + ".txt").
		// This matches what TruncatePathForBsdTar produces for long names.
		"ThisIsAVeryLongFileNameThatWillBeTruncatedByBsdTarToSixtyFou.txt": &fstest.MapFile{Data: []byte("truncated")},
	}

	tests := []struct {
		name          string
		input         string
		expectedPath  string
		expectedError bool
	}{
		{
			name:          "Exact Match",
			input:         "Folder/SubFolder/Document.pdf",
			expectedPath:  "Folder/SubFolder/Document.pdf",
			expectedError: false,
		},
		{
			name:          "Windows Slashes and Case Mismatch",
			input:         "folder\\SUBFOLDER\\document.PDF",
			expectedPath:  "Folder/SubFolder/Document.pdf",
			expectedError: false,
		},
		{
			name:          "Traverse Up with .. (Normal)",
			input:         "Folder/SubFolder/../SubFolder/Document.pdf",
			expectedPath:  "Folder/SubFolder/Document.pdf",
			expectedError: false,
		},
		{
			name:          "Traverse Above Root (Should stay at root)",
			input:         "../../Folder/SubFolder/Document.pdf",
			expectedPath:  "Folder/SubFolder/Document.pdf",
			expectedError: false,
		},
		{
			name: "BSD Tar Truncation Fallback",
			// Input is 73 chars. The function should mangle it to 64 chars and find the match.
			input:         "ThisIsAVeryLongFileNameThatWillBeTruncatedByBsdTarToSixtyFourCharsLong.txt",
			expectedPath:  "ThisIsAVeryLongFileNameThatWillBeTruncatedByBsdTarToSixtyFou.txt",
			expectedError: false,
		},
		{
			name:          "File Not Found (Returns Error)",
			input:         "Folder/MissingFile.txt",
			expectedPath:  "",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolvePathCaseInsensitive(mockFS, tt.input)

			if (err != nil) != tt.expectedError {
				t.Fatalf("For input %q, expected error: %v, but got: %v", tt.input, tt.expectedError, err)
			}

			if result != tt.expectedPath {
				t.Errorf("For input %q, expected path %q, but got %q", tt.input, tt.expectedPath, result)
			}
		})
	}
}

