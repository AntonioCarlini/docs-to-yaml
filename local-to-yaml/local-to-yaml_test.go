package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestParseIndirectFile(t *testing.T) {
	indirectFile, err := os.CreateTemp("", "docs-to-yaml-local-to-yaml*.txt")
	if err != nil {
		t.Fatalf("Cannot create temporary file")
	}
	fn := indirectFile.Name()
	fmt.Println("temp file = ", fn)
	indirectFile.Close()

	ok1_indirect := [][]string{{"/path/tree/file01.txt", "0001"}, {"/path/tree2/file02.txt", "0002"}, {"/path/tree3/file03.txt", "0003"}}
	err = CheckIndirectFileResponse(fn, ok1_indirect)

	ok2_indirect := [][]string{{"/path/tree/file01.txt", "0001", "/path/other/root"}, {"/path/tree2/file02.txt", "0002"}, {"/path/tree3/file03.txt", "0003"}}
	err = CheckIndirectFileResponse(fn, ok2_indirect)

	ok3_indirect := [][]string{{"/path/tree/file01.txt", "0001", "/path/other/root"}, {"\"/path/includes a space/file02.txt\"", "0002"}, {"/path/tree3/file03.txt", "0003"}}
	err = CheckIndirectFileResponse(fn, ok3_indirect)

	// Clear up by removing the temporary file
	os.Remove(fn)
}

func CheckIndirectFileResponse(indirectFilename string, data [][]string) error {
	indirectFile, err := os.OpenFile(indirectFilename, os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	for _, v := range data {
		text := v[0] + " " + v[1]
		indirectFile.WriteString(text + "\n")
	}
	indirectFile.Close()
	result := ParseIndirectFile(indirectFilename)
	if len(result) != len(data) {
		return fmt.Errorf("incoming data has %d elements, but result has %d", len(data), len(result))
	} else {
		for k, v := range result {
			root := filepath.Dir(data[k][0])
			if len(data[k]) >= 3 {
				root = data[k][2]
			}
			if (v.Path != data[k][0]) || (v.Volume != data[k][1]) || (v.Root != root) {
				return fmt.Errorf("mismatched result at entry %d", k)
			}
		}
	}
	return nil
}
