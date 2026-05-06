package persistentstore

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------

func tempFile(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "store.yaml")
}

// ---------------------------------------------------------------------
// Init tests
// ---------------------------------------------------------------------

func TestInit_WithCreate_Success(t *testing.T) {
	path := tempFile(t)
	var s Store[string, int]

	store, err := s.Init(path, true, false)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if store == nil {
		t.Fatal("Store is nil")
	}
	if !store.Active {
		t.Error("Active should be true")
	}
	if store.Dirty {
		t.Error("Dirty should be false after initialisation")
	}
	if store.Data == nil {
		t.Fatal("Data map is nil")
	}
	if len(store.Data) != 0 {
		t.Errorf("Expected empty store, got %d entries", len(store.Data))
	}
}

func TestInit_WithoutCreate_FileMissing(t *testing.T) {
	path := tempFile(t)
	var s Store[string, string]

	_, err := s.Init(path, false, false)
	if err == nil {
		t.Fatal("Expected error when file missing and create=false")
	}
}

func TestInit_WithoutCreate_FileExists(t *testing.T) {
	path := tempFile(t)
	// Create file with valid YAML
	if err := os.WriteFile(path, []byte("foo: bar\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var s Store[string, string]

	store, err := s.Init(path, false, false)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if !store.Active {
		t.Error("Active should be true")
	}
	if store.Data["foo"] != "bar" {
		t.Errorf("Expected foo=bar, got %v", store.Data["foo"])
	}
}

func TestInit_WithCreate_FileCorrupt(t *testing.T) {
	path := tempFile(t)
	if err := os.WriteFile(path, []byte("invalid: yaml: ["), 0644); err != nil {
		t.Fatal(err)
	}
	var s Store[string, string]

	_, err := s.Init(path, true, false)
	if err == nil {
		t.Fatal("Expected error due to corrupt YAML")
	}
}

func TestInit_WithCreate_FileExistsButEmpty(t *testing.T) {
	path := tempFile(t)
	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	var s Store[string, string]

	store, err := s.Init(path, true, false)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if len(store.Data) != 0 {
		t.Errorf("Expected empty map, got %d entries", len(store.Data))
	}
}

// ---------------------------------------------------------------------
// Lookup tests
// ---------------------------------------------------------------------

func TestLookup_Found(t *testing.T) {
	var s Store[int, string]
	store, err := s.Init("", false, false)
	if err != nil {
		t.Fatal(err)
	}
	store.Data[42] = "answer"

	val, ok := store.Lookup(42)
	if !ok {
		t.Error("Expected to find key 42")
	}
	if val != "answer" {
		t.Errorf("Expected 'answer', got '%s'", val)
	}
}

func TestLookup_NotFound(t *testing.T) {
	var s Store[string, bool]
	store, _ := s.Init("", false, false)

	val, ok := store.Lookup("missing")
	if ok {
		t.Error("Lookup should return false for missing key")
	}
	if val != false {
		t.Error("Zero value for bool is false, but got something else")
	}
}

// ---------------------------------------------------------------------
// Update tests
// ---------------------------------------------------------------------

func TestUpdate_AddsNewKey(t *testing.T) {
	var s Store[string, int]
	store, _ := s.Init("", false, false)

	store.Update("a", 100)
	if store.Data["a"] != 100 {
		t.Errorf("Expected 100, got %v", store.Data["a"])
	}
	if !store.Dirty {
		t.Error("Update should set Dirty to true")
	}
}

func TestUpdate_OverwritesExistingKey(t *testing.T) {
	var s Store[string, string]
	store, _ := s.Init("", false, false)
	store.Data["x"] = "old"

	store.Update("x", "new")
	if store.Data["x"] != "new" {
		t.Errorf("Expected 'new', got '%s'", store.Data["x"])
	}
	if !store.Dirty {
		t.Error("Overwriting should set Dirty to true")
	}
}

// ---------------------------------------------------------------------
// IsModified tests
// ---------------------------------------------------------------------

func TestIsModified_InitiallyFalse(t *testing.T) {
	var s Store[string, string]
	store, _ := s.Init("", false, false)
	if store.IsModified() {
		t.Error("IsModified should be false for new store")
	}
}

func TestIsModified_AfterUpdate(t *testing.T) {
	var s Store[string, string]
	store, _ := s.Init("", false, false)
	store.Update("key", "value")
	if !store.IsModified() {
		t.Error("IsModified should be true after Update")
	}
}

// Note: Save does NOT reset Dirty in the current implementation.
// Test this behaviour as-is.
func TestIsModified_AfterSave(t *testing.T) {
	path := tempFile(t)
	var s Store[string, string]
	store, _ := s.Init(path, true, false)

	store.Update("key", "value")
	if !store.IsModified() {
		t.Fatal("Expected Dirty true before Save")
	}

	store.Save(path)
	if !store.IsModified() {
		t.Error("Dirty remains true after Save (current implementation).")
	}
}

// ---------------------------------------------------------------------
// Save tests
// ---------------------------------------------------------------------

func TestSave_WritesCorrectYAML(t *testing.T) {
	path := tempFile(t)
	var s Store[string, int]
	store, _ := s.Init(path, true, false)

	// Use Update to ensure Dirty flag is set
	store.Update("one", 1)
	store.Update("two", 2)
	store.Save(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// yaml.v2 marshals map keys in sorted order, so "one" then "two"
	expected := "one: 1\ntwo: 2\n"
	if string(data) != expected {
		t.Errorf("Expected:\n%sGot:\n%s", expected, string(data))
	}
}

func TestSave_WithEmptyMap(t *testing.T) {
	path := tempFile(t)
	var s Store[string, string]
	store, _ := s.Init(path, true, false)

	store.Save(path)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Empty YAML could be "{}" or empty file depending on marshal.
	// yaml.Marshal on empty map returns "{}" as string.
	if string(data) != "{}\n" && string(data) != "" {
		t.Errorf("Expected empty YAML ('{}'), got '%s'", string(data))
	}
}

// ---------------------------------------------------------------------
// Persistence round-trip tests (generic types)
// ---------------------------------------------------------------------

func TestRoundTrip_StringKeysStringValues(t *testing.T) {
	path := tempFile(t)
	var s1 Store[string, string]
	store1, _ := s1.Init(path, true, false)

	store1.Update("apple", "fruit")
	store1.Update("carrot", "vegetable")
	store1.Save(path)

	var s2 Store[string, string]
	store2, err := s2.Init(path, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if val, ok := store2.Lookup("apple"); !ok || val != "fruit" {
		t.Errorf("Expected apple=fruit, got %v (found=%v)", val, ok)
	}
	if val, ok := store2.Lookup("carrot"); !ok || val != "vegetable" {
		t.Errorf("Expected carrot=vegetable, got %v", val)
	}
}

func TestRoundTrip_IntKeysStructValues(t *testing.T) {
	type Person struct {
		Name string
		Age  int
	}
	path := tempFile(t)
	var s1 Store[int, Person]
	store1, _ := s1.Init(path, true, false)

	store1.Update(1, Person{Name: "Alice", Age: 30})
	store1.Update(2, Person{Name: "Bob", Age: 25})
	store1.Save(path)

	var s2 Store[int, Person]
	store2, err := s2.Init(path, false, false)
	if err != nil {
		t.Fatal(err)
	}
	alice, ok := store2.Lookup(1)
	if !ok || alice.Name != "Alice" || alice.Age != 30 {
		t.Errorf("Expected Alice, got %+v", alice)
	}
	bob, ok := store2.Lookup(2)
	if !ok || bob.Name != "Bob" || bob.Age != 25 {
		t.Errorf("Expected Bob, got %+v", bob)
	}
}

func TestRoundTrip_OverwriteThenReload(t *testing.T) {
	path := tempFile(t)
	var s1 Store[string, int]
	store1, _ := s1.Init(path, true, false)
	store1.Update("count", 100)
	store1.Save(path)

	var s2 Store[string, int]
	store2, _ := s2.Init(path, false, false)
	if val, _ := store2.Lookup("count"); val != 100 {
		t.Fatalf("First load: expected 100, got %d", val)
	}

	// Overwrite in memory, save, reload again
	store2.Update("count", 200)
	store2.Save(path)

	var s3 Store[string, int]
	store3, _ := s3.Init(path, false, false)
	if val, _ := store3.Lookup("count"); val != 200 {
		t.Errorf("Second load: expected 200, got %d", val)
	}
}

// ---------------------------------------------------------------------
// Edge cases and concurrency (optional, simple)
// ---------------------------------------------------------------------

func TestInit_EmptyFilename(t *testing.T) {
	var s Store[string, string]
	store, err := s.Init("", true, false)
	if err != nil {
		t.Fatalf("Init with empty filename should not fail: %v", err)
	}
	if store.Active {
		t.Error("Active should be false when no filename given")
	}
	if store.Data == nil {
		t.Fatal("Data map should be initialised even without file")
	}
	if store.Dirty {
		t.Error("Dirty should be false")
	}
}

func TestSave_WithInactiveStoreDoesNothing(t *testing.T) {
	// Create store that is not Active (filename empty)
	var s Store[string, string]
	store, _ := s.Init("", false, false)
	store.Update("key", "value")
	store.Save("") // Should not panic, should not write.
	// No file to verify, just ensure no panic.
}

// ---------------------------------------------------------------------
// Bug detection: Dirty flag never reset
// ---------------------------------------------------------------------
// The following test documents current behaviour. If later the implementation
// resets Dirty after save, this test will need to be updated.
func TestDirtyFlagRemainsTrueAfterSave(t *testing.T) {
	path := tempFile(t)
	var s Store[string, string]
	store, _ := s.Init(path, true, false)
	store.Update("x", "y")
	store.Save(path)
	if !store.Dirty {
		t.Error("Expected Dirty to remain true after Save (current implementation)")
	}
}
