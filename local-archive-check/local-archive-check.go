package main

import (
	"bufio"
	"bytes"
	"docs-to-yaml/internal/document"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v2"
)

// The purpose of this program is to examine the root of a possible local archive tree and verify that all is in order.
//
// USAGE
//
// Run the program from the repo top level like this:
//
//   go run local-archive-check/local-archive-check.go --verbose --md5-cache bin/md5.store  --force-md5-sum --strict --tree-root ROOT
//
//  --verbose        turns on additional messages that may be useful in tracking program operation
//  --md5-cache      checks index.* MD5 checksums against those in the store
//  --force-md5-sum  causes MD5 checksums to be re-calculated
//  --tree-root      root of the tree which should be checked as a local archive
//  --fully-check    keep checking even in the face of severe errors to try to catch as many errors as possible; if not specified, stop on first fatal error
//
// NOTES
// md5sum
//    Must be present
//    Must represent every file (except perhaps index.*)
//    Optionally check every entry
// index.csv, index.yaml
//    Every character must be 7-bit ASCII
//    Check that .csv and .yaml match each other
//    Must represent every file
//    Must list an MD5 for every (file) entry
//    Optionally check every MD5 entry
//  index.html, index.pdf, index.txt should exist
//  No index.csv/.yaml other than at top level

type Document = document.Document

// Implement an enum for Metafile processing
type MetafileCategory int

// These are the legal ArchiveCategory enum values
const (
	MF_Undefined MetafileCategory = iota
	MF_CSV
	MF_YAML
	MF_MD5
)

type MetaFiles struct {
	path         string
	category     MetafileCategory
	present      bool
	correct      bool
	fileContents *[]byte
}

func main() {
	verbose := flag.Bool("verbose", false, "Enable verbose reporting")
	fullyCheck := flag.Bool("fully-check", false, "Continue in the face of errors")
	// forceMd5Gen := flag.Bool("force-md5-sum", false, "Enable generation of MD5 sums")
	treeRoot := flag.String("tree-root", "", "root of the tree for which YAML should be generated")
	// md5Storeilename := flag.String("md5-cache", "", "filepath of the file that holds the volume path => MD5sum map")

	flag.Parse()

	// Work out how long the root path is; this will be removed from the result to leave a relative path.
	// (Ensure that the prefix finishes with a /)
	treePrefix := *treeRoot
	if treePrefix[len(treePrefix)-1:] != "/" {
		treePrefix += "/"
	}
	treePrefixLength := len(treePrefix)

	// Check for the presence of critical meta files

	metafiles := []MetaFiles{
		{"index.csv", MF_CSV, false, false, nil},
		{"index.yaml", MF_YAML, false, false, nil},
		{"md5sums", MF_MD5, false, false, nil},
	}

	yamlDocumentsMap, csvRecords, md5Documents, err := HandleMetalFiles(treePrefix, metafiles)
	if err != nil {
		fmt.Println(err)
		if !*fullyCheck {
			log.Fatal("Stopping because of FATAL error.")
		}
	}

	// Accumulate the relative path to each file under the root, ignoring any directories.
	archiveDocumentsRelativeFilePaths := make(map[string]string)
	err = filepath.WalkDir(treePrefix, func(path string, d fs.DirEntry, err error) error {
		if !d.IsDir() {
			if path != "index.csv" && path != "index.yaml" {
				archiveDocumentsRelativeFilePaths[path[treePrefixLength:]] = path[treePrefixLength:]
			}
		}
		return nil
	})
	if err != nil {
		log.Fatalf("FATAL: impossible to walk directories: %s", err)
	}

	// TODO Temporary display of paths
	if *verbose {
		for _, doc := range archiveDocumentsRelativeFilePaths {
			fmt.Printf("INFO:  Found: %s\n", doc)
		}
	}

	// Verify that every file in the tree appears in the YAML and that every file in YAML appears in the tree
	// Verify that every file in the tree appears in the CSV and that every file in CSV appears in the tree
	// Verify that every file in the tree appears in the md5sum file and that every file in md5sum file appears in the tree

	// Start by building maps to make the checks simpler
	yamlDocsByPath := make(map[string]Document)
	for _, doc := range yamlDocumentsMap {
		yamlDocsByPath[doc.Filepath] = doc
	}

	csvDocsByPath := make(map[string]string)
	for _, record := range csvRecords {
		if record[0] == "Doc" {
			csvDocsByPath[record[2]] = record[6]
		}
	}

	filesRepresentedCorrectly := true

	// Verify the YAML file vs tree only if the YAML file is present (its absence will be noted as FATAL anyway)
	if len(yamlDocsByPath) > 0 {
		// Verify that every document in the tree appears in the YAML
		for _, docPath := range archiveDocumentsRelativeFilePaths {
			if _, present := yamlDocsByPath[docPath]; !present {
				if docPath != "index.csv" && docPath != "index.yaml" && docPath != "md5sums" {
					fmt.Printf("FATAL: Document missing from index.yaml: %s\n", docPath)
					filesRepresentedCorrectly = false
				}
			} else {
				if *verbose {
					fmt.Printf("INFO:  Document present in index.yaml: %s\n", docPath)
				}
			}
		}

		// Verify that every document listed in the YAML appears in the tree
		for _, doc := range yamlDocumentsMap {
			if _, present := archiveDocumentsRelativeFilePaths[doc.Filepath]; !present {
				fmt.Printf("FATAL: Document in index.yaml not present in file tree: %s\n", doc.Filepath)
				filesRepresentedCorrectly = false
			}
		}
	}

	// Verify the CSV file vs tree only if the CSV file is present (its absence will be noted as FATAL anyway)
	if len(csvDocsByPath) > 0 {
		// Verify that every document in the tree appears in the CSV
		for _, docPath := range archiveDocumentsRelativeFilePaths {
			if _, present := csvDocsByPath[docPath]; !present {
				if docPath != "index.csv" && docPath != "index.yaml" && docPath != "md5sums" {
					fmt.Printf("FATAL: Document missing from index.csv: %s\n", docPath)
					filesRepresentedCorrectly = false
				}
			} else {
				if *verbose {
					fmt.Printf("INFO:  Document present in index.csv: %s\n", docPath)
				}
			}
		}

		// Verify that every document in the CSV appears in the tree
		for path, _ := range csvDocsByPath {
			if _, present := archiveDocumentsRelativeFilePaths[path]; !present {
				fmt.Printf("FATAL: Document in index.csv not present in file tree: %s\n", path)
				filesRepresentedCorrectly = false
			}
		}
	}

	// Verify the md5sum file vs tree only if the md5sum file is present (its absence will be noted as FATAL anyway)
	if len(md5Documents) > 0 {
		// Verify that every document in the tree appears in the md5sum
		for _, docPath := range archiveDocumentsRelativeFilePaths {
			if _, present := md5Documents[docPath]; !present {
				// md5sums is expected to contain all files including metadata files, other than itself
				if docPath != "md5sums" {
					fmt.Printf("FATAL: Document missing from md5sum: %s\n", docPath)
					filesRepresentedCorrectly = false
				}
			} else {
				if *verbose {
					fmt.Printf("INFO:  Document present in md5sum: %s\n", docPath)
				}
			}
		}

		// Verify that every document in the md5sum file appears in the tree
		for path, _ := range md5Documents {
			if _, present := archiveDocumentsRelativeFilePaths[path]; !present {
				fmt.Printf("FATAL: Document in index.yaml not present in file tree: %s\n", path)
				filesRepresentedCorrectly = false
			}
		}
	}

	// Verify MD5 checksums between YAML and CSV
	if (len(yamlDocsByPath) > 0) && (len(csvDocsByPath) > 0) {
		fmt.Println("INFO:  Checking YAML vs CSV")
		for path, doc := range yamlDocsByPath {
			if csvDocMd5, present := csvDocsByPath[path]; !present {
				fmt.Printf("FATAL: checking YAML MD5 vs CSV MD5, document missing in CSV: %s\n", path)
				filesRepresentedCorrectly = false
			} else {
				if doc.Md5 != csvDocMd5 {
					fmt.Printf("FATAL: checking YAML MD5 vs CSV MD5, mismatch for: %s (YAML MD5=%s CSV MD5=%s\n", path, doc.Md5, csvDocMd5)
					filesRepresentedCorrectly = false
				}
			}
		}
	}

	// Verify MD5 checksums between YAML and md5sum
	if (len(yamlDocsByPath) > 0) && (len(md5Documents) > 0) {
		fmt.Println("INFO:  Checking YAML vs md5sum")
		for path, doc := range yamlDocsByPath {
			if md5Md5, present := md5Documents[path]; !present {
				fmt.Printf("FATAL: checking YAML MD5 vs md5sum MD5, document missing in md5sum: %s\n", path)
				filesRepresentedCorrectly = false
			} else {
				if doc.Md5 != md5Md5 {
					fmt.Printf("FATAL: checking YAML MD5 vs md5sum MD5, mismatch for: %s (YAML MD5=%s md5sum MD5=%s\n", path, doc.Md5, md5Md5)
					filesRepresentedCorrectly = false
				}
			}
		}

	}

	// Verify MD5 checksums between CSV and md5sum
	if (len(csvDocsByPath) > 0) && (len(md5Documents) > 0) {
		fmt.Println("INFO:  Checking CSV vs md5sum")
		for path, csvDocMd5 := range csvDocsByPath {
			if md5Md5, present := md5Documents[path]; !present {
				fmt.Printf("FATAL: checking CSV MD5 vs md5sum MD5, document missing in md5sum: %s\n", path)
				filesRepresentedCorrectly = false
			} else {
				if csvDocMd5 != md5Md5 {
					fmt.Printf("FATAL: checking YAML MD5 vs md5sum MD5, mismatch for: %s (YAML MD5=%s md5sum MD5=%s\n", path, csvDocMd5, md5Md5)
					filesRepresentedCorrectly = false
				}
			}

		}
	}

	if !filesRepresentedCorrectly {
		fmt.Println("FATAL: Some files missing from index or not present in tree")
		if !*fullyCheck {
			log.Fatal("Stopping because of FATAL error.")
		}
	}

	fmt.Printf("INFO:  Found (in YAML) %d documents\n", len(yamlDocumentsMap))

}

// A helper function that checks for possibly problematic characters
func HasProblematicCharacters(data *[]byte) bool {

	for _, ch := range *data {
		if ch > 0x7F {
			// At least one non-7-bit ASCII character found
			return false
		}
	}

	return true
}

// The metafiles include index.yaml and index.csv.
// This function reads them, performs some minimal sanity checks and
// then loads appropriate data to return to the caller.
func HandleMetalFiles(treePrefix string, metafiles []MetaFiles) (map[string]Document, [][]string, map[string]string, error) {

	documentsMap := make(map[string]Document)
	var csvRecords [][]string
	md5Map := make(map[string]string)

	var problematic_essential_files []string
	major_issue := false
	for _, mf := range metafiles {
		filePath := treePrefix + mf.path

		fileInfo, err := os.Stat(filePath)
		if err != nil {
			fmt.Printf("FATAL: Cannot stat %s\n", mf.path)
			major_issue = true
		} else {
			mode := fileInfo.Mode()
			if (mode&0200 != 0) || (mode&0020 != 0) || (mode&0002 != 0) {
				fmt.Printf("FATAL: Metafile is writeable %s (mode=%o)\n", mf.path, mode)
				major_issue = true
			}
		}
		content, err := os.ReadFile(filePath)
		if err == nil {
			mf.present = true
			mf.correct = true
			mf.fileContents = &content
			if !HasProblematicCharacters(mf.fileContents) {
				mf.correct = false
				fmt.Printf("FATAL: Metafile with non-ASCII characters: %s\n", mf.path)
				major_issue = true
			} else {
				// Apply special processing
				switch mf.category {
				case MF_YAML:
					err = yaml.Unmarshal(*mf.fileContents, &documentsMap)
					if err != nil {
						fmt.Printf("FATAL: YAML unmarshal error for %s: %v", mf.path, err)
						major_issue = true
					}
				case MF_CSV:
					reader := csv.NewReader(bytes.NewReader(*mf.fileContents))

					// Read all the records from the CSV
					csvRecords, err = reader.ReadAll()
					if err != nil {
						fmt.Printf("FATAL: CSV record reading error for %s: %v", mf.path, err)
						major_issue = true
					}
					// TODO perform minimal sanity checks: e.g. header record as expected
				case MF_MD5:
					// A line from md5sum should look like this:
					// 4556f5bdf78aa195b18e06e35a64c89f *mvxaaig1.pdf
					// That's exactly 32 characters of md5 checksum, a space, either a space or an asterisk and finally a filepath (relative to the md5sum)
					// The asterisk is present if the checksum was generated in binary mode; on my Linux system the result is the same whether binary mode is selected or not.
					md5Regex := regexp.MustCompile(`^([a-f0-9]{32})\s(?:\s|\*)(.+)$`)
					scanner := bufio.NewScanner(bytes.NewReader(*mf.fileContents))
					lineCount := 0
					for scanner.Scan() {
						line := scanner.Text()
						lineCount += 1
						// Match the line using the regex
						matches := md5Regex.FindStringSubmatch(line)
						if matches == nil {
							fmt.Printf("FATAL: md5sum invalid format on line %d: %s", lineCount, line)
							major_issue = true
						}

						md5sum := matches[1]
						filepath := matches[2]
						md5Map[filepath] = md5sum
					}
					if err := scanner.Err(); err != nil {
						fmt.Printf("FATAL: md5sum record reading error for %s: %v", mf.path, err)
						major_issue = true
					}
				case MF_Undefined:
				}

			}
		} else {
			fmt.Printf("FATAL: Cannot read %s: %v\n", mf.path, err)
			problematic_essential_files = append(problematic_essential_files, mf.path)
			major_issue = true
		}

	}

	if len(problematic_essential_files) > 0 {
		fmt.Println("FATAL: Missing essential file(s): ", strings.Join(problematic_essential_files, ","))
	}

	if major_issue {
		return documentsMap, csvRecords, md5Map, errors.New("FATAL error checking essential metadata files")
	} else {

		return documentsMap, csvRecords, md5Map, nil
	}
}
