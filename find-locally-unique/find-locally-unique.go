package main

// The purpose of this program is to accept a set of files that a present locally and a set of files
// that are known to exist in repositories available on the internet and to produce an output
// consisting of files present locally that are unique.
//
// To determine that a file is a duplicate the following rules are used:
// = files with identical MD5 sums are considered identical
// = local files with certain strings in their filepaths are considered to have originated from an internet repository,
//   for example a local file with "/bitsavers/" will not be considered unique
// = any local file whose part # matches that of a remote document will will not be considered unique
// = any local file whose filename matches that of a remote document will will not be considered unique
//
// Any local documents not filtered out by this processing will end up in the final YAMl file.
// This file can then form the basis of further processing to produce a candidate list of files
// to be made available to remote repositories, along with appropriate metdadata.

import (
	"docs-to-yaml/internal/document"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"
)

type Document = document.Document

// Main entry point.
// Processes the indirect file.
// For each entry, parses the specified HTML file.
// Finally outputs the cumulative YAML file.
func main() {
	localYamlFiles := make([]string, 0)
	remoteYamlFiles := make([]string, 0)

	uniqueDocuments := make(map[string]Document)
	flag.Func("local", "specify a set of YAML files describing local documents", func(s string) error {
		fmt.Println("called local with ", s)
		localYamlFiles = append(localYamlFiles, s)
		return nil
	})

	flag.Func("remote", "specify a set of YAML files describing remote documents", func(s string) error {
		fmt.Println("called remote with ", s)
		remoteYamlFiles = append(remoteYamlFiles, s)
		return nil
	})

	verbose := flag.Bool("verbose", false, "Enable verbose reporting")
	yamlOutputFilename := flag.String("yaml", "", "filepath of the output file to hold the generated yaml")

	flag.Parse()

	writeOutputYaml := (*yamlOutputFilename != "")
	logLocallyUniqueFiles := *verbose || !writeOutputYaml
	fmt.Printf("output YAML: [%s] write yaml: %t verbose: %t\n", *yamlOutputFilename, writeOutputYaml, *verbose)
	// Build list of all remote files
	localDocuments := BuildMapOfDocuments(localYamlFiles)
	remoteDocuments := BuildMapOfDocuments(remoteYamlFiles)
	if *verbose {
		fmt.Println("Found ", len(localDocuments), "local documents")
		fmt.Println("Found ", len(remoteDocuments), "remote documents")
	}

	var mapRemoteDocsByPartNum map[string]Document = make(map[string]Document)
	var mapRemoteDocsByFilename map[string]Document = make(map[string]Document)

	// Build maps of remote documents by filename (not filepath) and by part number
	for _, v := range remoteDocuments {
		partNum := v.PartNum
		partNum = strings.Replace(partNum, "-", "", -1)
		partNum = strings.Replace(partNum, ".", "", -1)
		if _, found := mapRemoteDocsByPartNum[partNum]; found {
			if *verbose {
				fmt.Printf("WARNING: non-unique Part Num %s (was %s) for %s and %s - dropped latter\n", partNum, v.PartNum, mapRemoteDocsByPartNum[v.PartNum].Filepath, v.Filepath)
			}
		} else {
			mapRemoteDocsByPartNum[partNum] = v
		}
		fn := filepath.Base(v.Filepath)
		if _, found := mapRemoteDocsByFilename[fn]; found {
			if *verbose {
				fmt.Printf("WARNING: non-unique filename %s for %s and %s - dropped latter\n", fn, v.Filepath, mapRemoteDocsByFilename[fn].Filepath)
			}
		} else {
			mapRemoteDocsByFilename[fn] = v
		}
	}

	// For each local document, look it's MD5 up in the remote set and report any that are not found.
	// Also report any local document with an empty MD5 string
	localMissingMd5 := 0
	locallyUnique := 0
	matchedPN := 0
	matchedFN := 0
	matchedPath := 0
	matchedMD5 := 0

	partialPathsToReject := []string{"/metadata/", "/bitsavers/", "/chook/", "/MDS/1994-"}

	for _, localDoc := range localDocuments {
		if localDoc.Md5 == "" {
			fmt.Printf("Local MD5 missing:  %s\n", localDoc.Filepath)
			localMissingMd5 += 1
		}

		// Reject any document that contains any of the partial paths in its own filepath
		rejectPartialPath := false
		for _, p := range partialPathsToReject {
			if strings.Contains(localDoc.Filepath, p) {
				// Skip any file with bitsavers as a path element as it almost certainly came from an existing remote source in the first place
				matchedPath += 1
				rejectPartialPath = true
				break
			}
		}
		if rejectPartialPath {
			continue
		}

		// Reject any local document that exactly matches a remote document's MD5 checksum
		if _, found := remoteDocuments[localDoc.Md5]; found {
			matchedMD5 += 1
			continue
		}

		// Reject any document that matches a remote document's DEC part number
		partNum := localDoc.PartNum
		partNum = strings.Replace(partNum, "-", "", -1)
		partNum = strings.Replace(partNum, ".", "", -1)
		if _, foundPN := mapRemoteDocsByPartNum[partNum]; foundPN {
			matchedPN += 1
			continue
		}

		// Reject any document that matches a remote document's filename
		if _, found := mapRemoteDocsByFilename[filepath.Base(localDoc.Filepath)]; found {
			matchedFN += 1
			continue
		}

		// Here unique document found
		if logLocallyUniqueFiles {
			fmt.Printf("Not found remotely: %s\n", localDoc.Filepath)
		}
		uniqueDocuments[localDoc.Filepath] = localDoc
		locallyUnique += 1
	}

	fmt.Printf("Local files with missing MD5 checksum: %d\n", localMissingMd5)
	fmt.Printf("Local files dropped by MD5:            %d\n", matchedMD5)
	fmt.Printf("Local files dropped by path portion:   %d\n", matchedPath)
	fmt.Printf("Local files dropped by part number:    %d\n", matchedPN)
	fmt.Printf("Local files dropped by filename:       %d\n", matchedFN)
	fmt.Printf("Local files that are unique:           %d\n", locallyUnique)

	// Write the output YAML file
	if writeOutputYaml {
		data, err := yaml.Marshal(&uniqueDocuments)
		if err != nil {
			log.Fatal("Bad YAML data: ", err)
		}

		err = os.WriteFile(*yamlOutputFilename, data, 0644)
		if err != nil {
			log.Fatal("Failed YAML write: ", err)
		}
	}
}

func YamlDataInit(filename string) (map[string]Document, error) {
	documents := make(map[string]Document)

	file, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return documents, nil
		} else {
			return documents, err
		}
	}
	// Read the existing cache YAML data into the cache
	err = yaml.Unmarshal(file, documents)
	if err != nil {
		fmt.Println("YAML: failed to unmarshal")
		return documents, err
	}
	fmt.Printf("Initial  number of YAML entries in %s: %d\n", filename, len(documents))
	return documents, err
}

// Build a map of "key => Document"
// where key is a string that is the MD5 checksum, if any, otherwise
// use the part number or title or filepath.
func BuildMapOfDocuments(filenames []string) map[string]Document {
	documents := make(map[string]Document, 0)

	for _, names := range filenames {
		// Start by reading the output yaml file.
		initialData, err := YamlDataInit(names)
		if err != nil {
			log.Fatal(err)
		}

		// Loop through the new documents, adding them to the master list
		for k, v := range initialData {
			// Pick an appropriate key, defaulting to the MD5 value
			key := k
			if (key != v.Md5) && (v.Md5 != "") {
				key = v.Md5
			} else if key == "" {
				key = document.BuildKeyFromDocument(v)
			}

			// If the key is already known and all other aspects of the document are the same, ignore as a genuine duplicate
			if existing, found := documents[key]; found {
				if v.Md5 != existing.Md5 {
					fmt.Println("Found presumed-smae docs with the differing MD5: ", v, " and ", existing)
				} else {
					// Drop the duplicate silently
				}
			} else {
				documents[key] = v
			}
		}
	}

	return documents
}
