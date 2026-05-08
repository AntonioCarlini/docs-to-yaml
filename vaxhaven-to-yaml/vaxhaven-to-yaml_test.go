package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestCreateVaxHavenDocument(t *testing.T) {
	doc := CreateVaxHavenDocument("http://www.vaxhaven.com/images/test.pdf")

	if doc.PublicUrl != "http://www.vaxhaven.com/images/test.pdf" {
		t.Errorf("Expected PublicUrl to be set, got %s", doc.PublicUrl)
	}
	if doc.Collection != "VaxHaven" {
		t.Errorf("Expected Collection to be 'VaxHaven', got %s", doc.Collection)
	}
	if doc.Size != 0 {
		t.Errorf("Expected Size to initialize to 0, got %d", doc.Size)
	}
}

func TestConvertVaxHavenDate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Standard format", "1977 April", "1977-04"},
		{"Standard format mixed case", "1982 december", "1982-12"},
		{"Year only", "1999", "1999"},
		{"Blank string", "", ""},
		{"Hyphen", "-", ""},
		{"Too short", "123", "XXXX"},
		{"Not a year string", "ABCD", "XXXX"},
		{"Month only (bad format)", "198 M", "XXXX"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertVaxHavenDate(tt.input)
			if result != tt.expected {
				t.Errorf("ConvertVaxHavenDate(%q) = %q; expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCalculatefileSize_CacheHit(t *testing.T) {
	// Pre-populate the cache to avoid network calls and the hardcoded time.Sleep
	store := &Store{
		Data: map[string]int64{
			"http://example.com/doc.pdf": 9999,
		},
	}

	size, err := CalculatefileSize("http://example.com/doc.pdf", store, false)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if size != 9999 {
		t.Errorf("Expected size 9999 from cache, got %d", size)
	}
}

func TestCalculatefileSize_CacheMiss(t *testing.T) {
	// Create a mock HTTP server to intercept the HEAD request safely
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", "54321")
			w.WriteHeader(http.StatusOK)
		} else {
			t.Errorf("Expected HEAD request, got %s", r.Method)
		}
	}))
	defer mockServer.Close()

	store := &Store{
		Data: make(map[string]int64),
	}

	// This will trigger the 2-second sleep in the original code, but it's safe
	// because the mock server prevents an actual outbound internet connection.
	size, err := CalculatefileSize(mockServer.URL, store, false)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if size != 54321 {
		t.Errorf("Expected size 54321 from HTTP HEAD, got %d", size)
	}

	// Verify the store was updated
	if val, ok := store.Lookup(mockServer.URL); !ok || val != 54321 {
		t.Errorf("Expected store to be updated with 54321, got %d", val)
	}
}

func TestParseNewData(t *testing.T) {
	// 1. Create a mock HTML file with rows that match both regex paths
	mockHTML := `
	<html><body>
	<table>
		<tr>
			<td><a href="/images/1/11/doc1.pdf" class="internal">PART-123</a></td>
			<td>Title One</td>
			<td>1980 May</td>
		</tr>
		<tr>
			<td><a href="/images/2/22/doc2.pdf" class="internal">PART-456</a></td>
			<td>Title Two</td>
		</tr>
		<tr>
			<td><a href="/images/1/11/doc1.pdf" class="internal">PART-123</a></td>
			<td>Title One</td>
			<td>1980 May</td>
		</tr>
	</table>
	</body></html>`

	tmpFile, err := os.CreateTemp("", "vaxhaven_mock_*.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name()) // Clean up

	if _, err := tmpFile.Write([]byte(mockHTML)); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	// 2. Pre-populate the cache for the URLs so the parser doesn't trigger
	// the hardcoded 2-second HTTP sleep for every document found.
	store := &Store{
		Data: map[string]int64{
			vaxhaven_prefix + "/images/1/11/doc1.pdf": 1000,
			vaxhaven_prefix + "/images/2/22/doc2.pdf": 2000,
		},
	}

	// 3. Execute the parser
	docs := ParseNewData(tmpFile.Name(), store, false)

	// 4. Assertions
	if len(docs) != 2 {
		t.Fatalf("Expected 2 unique documents, got %d", len(docs))
	}

	doc1, ok1 := docs["PART-123"]
	if !ok1 {
		t.Fatalf("Expected PART-123 to be parsed")
	}
	if doc1.Title != "Title One" || doc1.PubDate != "1980-05" || doc1.Size != 1000 {
		t.Errorf("PART-123 data incorrect: %+v", doc1)
	}

	doc2, ok2 := docs["PART-456"]
	if !ok2 {
		t.Fatalf("Expected PART-456 to be parsed")
	}
	if doc2.Title != "Title Two" || doc2.PubDate != "" || doc2.Size != 2000 {
		t.Errorf("PART-456 data incorrect: %+v", doc2)
	}
}
