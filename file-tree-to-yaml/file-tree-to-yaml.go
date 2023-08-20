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
	"errors"
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
	fnfList := flag.Bool("fnf-list", false, "Report file not found")
	fnfDiscard := flag.Bool("fnf-discard", false, "Report file not found")
	yamlOutputFilename := flag.String("yaml", "", "filepath of the output file to hold the generated yaml")
	md5Gen := flag.Bool("md5-sum", false, "Enable generation of MD5 sums")
	exifRead := flag.Bool("exif", false, "Enable EXIF reading")
	treeRoot := flag.String("tree-root", "", "root of the tree for which YAML should be generated")

	flag.Parse()

	if *yamlOutputFilename == "" {
		log.Fatal("Please supply a filespec for the output YAML")
	}

	// Start by reading the output yaml file.
	initialData, err := YamlDataInit(*yamlOutputFilename)
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

	var mapByMd5 map[string]Document = make(map[string]Document)
	var mapByFilepath map[string]Document = make(map[string]Document)

	for _, v := range initialData {
		md5 := v.Md5
		if md5 == "" {
			md5 = v.Filepath
		}
		if _, found := mapByMd5[md5]; found {
			fmt.Printf("WARNING: non-unique MD5 %s for %s and %s - dropped latter\n", v.Md5, mapByMd5[v.Md5].Filepath, v.Filepath)
		} else {
			mapByMd5[md5] = v
		}

		if _, found := mapByFilepath[v.Filepath]; found {
			fmt.Printf("WARNING: non-unique filepath %s for %s and %s - dropped latter\n", v.Filepath, mapByMd5[v.Filepath].Filepath, v.Filepath)
		} else {
			mapByFilepath[v.Filepath] = v
		}
	}

	if len(mapByMd5) != len(mapByFilepath) {
		log.Fatalf("After all processing, MD5 and Filepath maps are different sizes; %d docs listed by MD5 and %d listed by filepath\n", len(mapByMd5), len(mapByFilepath))
	} else {
		fmt.Printf("After loading and processing YAML file, %d documents are known.\n", len(mapByFilepath))
	}

	for _, filepath := range paths {
		doc, found := mapByFilepath[filepath]
		if !found {
			doc = CreateLocalDocument(filepath)
		}
		originalMd5 := doc.Md5

		// Set up properties that are determined by the filepath, but only if they are currently missing
		data := document.DetermineDocumentPropertiesFromPath(doc.Filepath, true)
		if doc.Format == "" {
			doc.Format = data.Format
		}
		if doc.Title == "" {
			doc.Title = data.Title
			document.SetFlags(doc, "T")
		}
		if doc.PartNum == "" {
			doc.PartNum = data.PartNum
			document.SetFlags(doc, "P")
		}
		if doc.PubDate == "" {
			doc.PubDate = data.PubDate
			document.SetFlags(doc, "D")
		}

		fullPath := treePrefix + doc.Filepath

		// Calculate the MD5 checksum if requested and not already present
		if *md5Gen {
			if (doc.Md5 == "") && (doc.Md5 != doc.Filepath) {
				if *verbose {
					fmt.Println("Calculating MD5 for ", fullPath)
				}
				fileBytes, err := os.ReadFile(fullPath)
				if err != nil {
					log.Fatalf("Cannot compute MD5 for %s: %s", fullPath, err)
				}
				md5Hash := md5.Sum(fileBytes)
				md5Checksum := hex.EncodeToString(md5Hash[:])
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

		// Update the map entry in case it has changed
		mapByFilepath[filepath] = doc
		// MD5 checksum may have changed: if so, remove the old entry from the map keyed on MD5 checksum
		if originalMd5 != doc.Md5 {
			delete(mapByMd5, originalMd5)
		}
		mapByMd5[doc.Md5] = doc
	}

	// Ensure that each document is listed
	fmt.Println("Finished with this many documents: ", len(mapByFilepath))

	if *fnfList || *fnfDiscard {
		for k, d := range mapByFilepath {
			fullPath := treePrefix + d.Filepath
			if d.Filepath == "" && (k != d.Md5) {
				fullPath = treePrefix + k
			}
			fmt.Println("checking ", fullPath)
			if _, err := os.Stat(fullPath); errors.Is(err, os.ErrNotExist) {
				if *fnfList {
					fmt.Println("Non-existent file found in YAML    :", fullPath)
				}
				if *fnfDiscard {
					delete(mapByFilepath, k)
					if md5Entry, found := mapByMd5[d.Md5]; !found {
						fmt.Println("cannot delete entry from MD5 map (not found): ", fullPath)
					} else if md5Entry.Filepath == d.Filepath {
						delete(mapByMd5, d.Md5)
						fmt.Println("Deleted entry from MD5 map: ", fullPath)
					} else {
						fmt.Println("Must not delete entry from MD5 map (diff doc): ", fullPath)

					}
					fmt.Println("Non-existent file removed from YAML:", fullPath)
				}
			}
		}
		fmt.Println("Finally finished with this many documents: ", len(mapByFilepath))
	}

	// After all the manipualtion, there must be exactly the same number of documents in the MD5 and Filepath maps
	if len(mapByMd5) != len(mapByFilepath) {
		log.Fatalf("After all final processing, MD5 and Filepath maps are different sizes; %d docs listed by MD5 and %d listed by filepath\n", len(mapByMd5), len(mapByFilepath))
	}

	// Write the output YAML file
	data, err := yaml.Marshal(&mapByMd5)
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
