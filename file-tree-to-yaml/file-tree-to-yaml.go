package main

// This program works through a tree of files, starting at a specified root,
// and produces a YAML file that describes the contents.
//
// The intention is to do the bulk of the work necessary to produce a YAML
// file that describes each document's part number, title, date, MD5 checksum, etc.

// This is currently a work in progress.

import (
	"crypto/md5"
	"docs-to-yaml/internal/document"
	"docs-to-yaml/internal/pdfmetadata"
	"encoding/hex"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v2"
)

type Document = document.Document

type PdfMetadata = pdfmetadata.PdfMetadata

// PathAndVolume is used when parsing the indirect file
type PathAndVolume struct {
	Path   string
	Volume string
}

// Md5Cache records information about the MD5 cache itself.
type Md5Cache struct {
	Active           bool              // True if the cache is in use
	Dirty            bool              // True if the cache has been modified (and should be written out)
	CacheOfPathToMd5 map[string]string // A cache of path => computed MD5 sum
}

// Main entry point.
// Processes the indirect file.
// For each entry, parses the specified HTML file.
// Finally outputs the cumulative YAML file.
func main() {
	verbose := flag.Bool("verbose", false, "Enable verbose reporting")
	yamlOutputFilename := flag.String("yaml", "", "filepath of the output file to hold the generated yaml")
	md5Gen := flag.Bool("md5-sum", false, "Enable generation of MD5 sums")
	exifRead := flag.Bool("exif", false, "Enable EXIF reading")
	treeRoot := flag.String("tree-root", "", "root of the tree for which YAML should be generated")

	flag.Parse()

	if *yamlOutputFilename == "" {
		log.Fatal("Please supply a filespec for the output YAML")
	}

	// Start by reading the output yaml file.
	documents, err := YamlDataInit(*yamlOutputFilename)
	if err != nil {
		log.Fatal(err)
	}

	// The documents map is keyed on the path

	// If none, then create an empty map.
	// Otherwise read the YAML into the map.
	// Now work through each file in the specified tree root.
	// If its path already exists then
	//  update MD5 if requested and not already specified
	//  update PDF data if requested and not already specified

	// Work out how long the root path is; this will be removed from the result to leave a relative path.
	// (Ensure that the prefix finishes with a /)
	treePrefix := *treeRoot
	if treePrefix[len(treePrefix)-1:] != "/" {
		treePrefix += "/"
	}
	treePrefixLength := len(treePrefix)

	// Accumulate the path to each file under the root, ignoring any directories.
	var paths []string
	err = filepath.WalkDir(*treeRoot, func(path string, d fs.DirEntry, err error) error {
		if !d.IsDir() {
			paths = append(paths, path[treePrefixLength:])
		}
		return nil
	})
	if err != nil {
		log.Fatalf("impossible to walk directories: %s", err)
	}

	for _, filepath := range paths {
		doc, found := documents[filepath]
		if !found {
			doc = CreateLocalDocument(filepath)

		}
		fullPath := treePrefix + doc.Filepath

		// Calculate the MD5 checksum if requested and not already present
		if *md5Gen {
			if doc.Md5 == "" {
				if *verbose {
					fmt.Println("Calculating MD5 for ", fullPath)
				}
				fileBytes, err := os.ReadFile(fullPath)
				if err != nil {
					log.Fatalf("Cannot compute MD5 for %s: %s", fullPath, err)
				}
				md5Hash := md5.Sum(fileBytes)
				md5Checksum := hex.EncodeToString(md5Hash[:])
				doc = documents[filepath]
				doc.Md5 = md5Checksum
			}
		}

		// Read the EXIF data if requested and any of it is missing
		// TOOD only do this if the format is PDF!
		if *exifRead {
			if (doc.PdfCreator == "") || (doc.PdfProducer == "") || (doc.PdfVersion == "") || (doc.PdfModified == "") {
				pdfMetadata := pdfmetadata.ExtractPdfMetadata(fullPath)

				doc.PdfCreator = pdfMetadata.Creator
				doc.PdfProducer = pdfMetadata.Producer
				doc.PdfVersion = pdfMetadata.Format
				doc.PdfModified = pdfMetadata.Modified
			}
		}

		// Query the file size, unless it is already known
		if doc.Size == 0 {
			filestats, err := os.Stat(fullPath)
			if err != nil {
				log.Fatal(err)
			}
			doc.Size = filestats.Size()
		}

		// A number of further items are still on the TODO list
		// newDocument.Format = DetermineFileFormat(volumePath) TODO should be handled by creator function
		// doc.Title = title
		// doc.PartNum = partNumber
		// doc.PubDate = date

		// Update the map entry in case it has changed
		documents[filepath] = doc
	}

	fmt.Println("Finished with this many documents: ", len(documents))

	// Write the output YAML file
	data, err := yaml.Marshal(&documents)
	if err != nil {
		log.Fatal("Bad YAML data: ", err)
	}

	err = os.WriteFile(*yamlOutputFilename, data, 0644)
	if err != nil {
		log.Fatal("Failed YAML write: ", err)
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
	fmt.Printf("Initial  number of YAML entries: %d\n", len(documents))
	return documents, err
}

// This function function creates a Document struct with some default values set
func CreateLocalDocument(path string) Document {
	var newDocument Document
	newDocument.Md5 = ""
	newDocument.PubDate = ""
	newDocument.PdfCreator = ""
	newDocument.PdfProducer = ""
	newDocument.PdfVersion = ""
	newDocument.PdfModified = ""
	newDocument.Collection = "local"
	newDocument.Size = 0
	newDocument.Filepath = path

	return newDocument
}
