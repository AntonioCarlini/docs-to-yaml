package main

import (
	"docs-to-yaml/internal/document"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
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

	if *verbose {
		fmt.Printf("Program started\n")
	}

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
		{"md5sum", MF_MD5, false, false, nil},
	}

	documentsMap := make(map[string]Document)

	var problematic_essential_files []string
	major_issue := false
	for _, mf := range metafiles {
		content, err := os.ReadFile(treePrefix + mf.path)
		if err == nil {
			mf.present = true
			mf.correct = true
			mf.fileContents = &content
			if !HasProblematicCharacters(mf.fileContents) {
				mf.correct = false
				fmt.Printf("Metafile with non-ASCII characters: %s\n", mf.path)
			} else {
				// Apply special processing
				switch mf.category {
				case MF_YAML:
					err = yaml.Unmarshal(*mf.fileContents, &documentsMap)
					if err != nil {
						log.Fatalf("YAML unmarshal error for %s: %v", mf.path, err)
					}
				case MF_CSV:
				case MF_MD5:
				case MF_Undefined:
				}

			}
		} else {
			fmt.Printf("Cannot find %s: %v\n", mf.path, err)
			problematic_essential_files = append(problematic_essential_files, mf.path)
			major_issue = true
		}

	}

	if !*fullyCheck && major_issue {
		log.Fatal("Missing essential file(s): ", strings.Join(problematic_essential_files, ","))
	}

	// Accumulate the path to each file under the root, ignoring any directories.
	var archiveDocumentsRelativeFilePaths []string
	err := filepath.WalkDir(treePrefix, func(path string, d fs.DirEntry, err error) error {
		if !d.IsDir() {
			if path != "index.csv" && path != "index.yaml" {
				archiveDocumentsRelativeFilePaths = append(archiveDocumentsRelativeFilePaths, path[treePrefixLength:])
			}
		}
		return nil
	})
	if err != nil {
		log.Fatalf("impossible to walk directories: %s", err)
	}

	for _, doc := range archiveDocumentsRelativeFilePaths {
		fmt.Printf("Found: %s\n", doc)
	}

	fmt.Printf("Found (in YAML) %d documents\n", len(documentsMap))

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
