package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	err = CheckIndirectFileResponse(fn, ok1_indirect, false)
	if err != nil {
		t.Fatalf("Failed ParseIndirectFile(ok1_indirect) = %s", err)
	}

	ok2_indirect := [][]string{{"/path/tree/file01.txt", "0001", "/path/other/root"}, {"/path/tree2/file02.txt", "0002"}, {"/path/tree3/file03.txt", "0003"}}
	err = CheckIndirectFileResponse(fn, ok2_indirect, false)
	if err != nil {
		t.Fatalf("Failed ParseIndirectFile(ok2_indirect) = %s", err)
	}

	ok3_indirect := [][]string{{"/path/tree/file01.txt", "0001", "/path/other/root"}, {"\"/path/includes a space/file02.txt\"", "0002"}, {"/path/tree3/file03.txt", "0003"}}
	err = CheckIndirectFileResponse(fn, ok3_indirect, false)
	if err != nil {
		t.Fatalf("Failed ParseIndirectFile(ok3_indirect) = %s", err)
	}

	// Line 2 has only one value
	fail1_indirect := [][]string{{"/path/tree/file01.txt", "0001", "/path/other/root"}, {"/path/tree2/file02.txt"}, {"/path/tree3/file03.txt", "0003"}}
	err = CheckIndirectFileResponse(fn, fail1_indirect, true)
	if err != nil {
		t.Fatalf("Failed ParseIndirectFile(fail1_indirect) = %s", err)
	}

	// Clear up by removing the temporary file
	os.Remove(fn)
}

func CheckIndirectFileResponse(indirectFilename string, data [][]string, expectError bool) error {
	indirectFile, err := os.OpenFile(indirectFilename, os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	for _, v := range data {
		text := strings.Join(v, " ")
		indirectFile.WriteString(text + "\n")
	}
	indirectFile.Close()

	result, err := ParseIndirectFile(indirectFilename)
	if expectError && (err == nil) {
		return fmt.Errorf("Expected error but ParseIndirectFile() returned success")
	} else if !expectError && (err != nil) {
		return fmt.Errorf("Expected success but ParseIndirectFile() returned error: %s", err)
	}

	// If an error has been signalled, there's no point checking the data itself.
	// We also do not check the nature of the error: that there has been an error signalled is enough of a test.
	if err != nil {
		return nil
	}

	if len(result) != len(data) {
		return fmt.Errorf("incoming data has %d elements, but result has %d; err=%s; data in = %#v", len(data), len(result), err, data)
	} else {
		for k, v := range result {
			path := ""
			volume := ""
			root := ""
			switch len(data[k]) {
			case 0:
			case 1:
				path = data[k][0]
				root = filepath.Dir(path)
			case 2:
				path = data[k][0]
				volume = data[k][1]
				root = filepath.Dir(data[k][0])
			case 3:
				path = data[k][0]
				volume = data[k][1]
				root = data[k][2]
			}
			// If resulting path includes a leading and final double quote remove them.
			// In this case also remove a leading double quote from root, if one is present.
			if (path[0] == '"') && (path[len(path)-1] == '"') {
				path = path[1 : len(path)-1]
				if root[0] == '"' {
					root = root[1:]
				}
			}
			if (v.Path != path) || (v.Volume != volume) || (v.Root != root) {
				return fmt.Errorf("mismatched result at entry %d: {%s},{%s},{%s} != {%s},{%s},{%s}", k, v.Path, v.Volume, v.Root, path, volume, root)
			}
		}
	}
	return nil

}
