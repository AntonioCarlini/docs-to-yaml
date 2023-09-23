package main

import (
	"bufio"
	"docs-to-yaml/internal/document"
	"docs-to-yaml/internal/persistentstore"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// This program takes the bitsavers IndexByDate.txt file and produces a YAML output that describes each entry.
// By default the input file is expected to be in the data/ subdirectory and named bitsavers-IndexByDate.txt.
// Currently only dec/, able/, dilog, emulex/, mentec/ and terak/ areas are included.
//
// The IndexByDate.txt file does not contain any MD5 data. However the maintainer of manx supplied such
// data and that is used to fill in the missing MD5 data, which is to be found in site.bitsavers.2021-10-01.md5.
//
// Note that currently no command line arguments are accepted, so the "defaults" above are hard-coded!

// No need to do any sort of title deduction here as all files will be local sooner or later and those will have a proper title.

// Operation
// For each line in the IndexByDate.txt:
//   reject lines with uninteresting file types
//   keep remaining paths in an array of strings
// For each acceptable path:
//   Remove the file type and use it for the Format field
//   Parse out a part number (everything before the first underscore)
//   The remainder is a provisional title
//   If there is a trailing date (e.g. _Jan91) remove it from the title and put it in the PubDate field
//   If the path matches one in the supplied MD5 file, put that in the Md5 field

// ISSUES:
//  o The part number could do with some sanity checks
//

type Document = document.Document

var bitsavers_prefix = "http://bitsavers.org/pdf/"

func main() {

	var docs []string

	bitsavers_index_filename := "data/bitsavers-IndexByDate.txt"
	bitsavers_md5_filename := "data/site.bitsavers.2021-10-01.md5"
	output_file := "bitsavers.yaml"
	verbose := false
	md5CacheFilename := "bin/md5.store"
	md5CacheCreate := false

	md5StoreInstantiation := persistentstore.Store[string, string]{}
	md5Store, err := md5StoreInstantiation.Init(md5CacheFilename, md5CacheCreate, verbose)
	if err != nil {
		fmt.Printf("Problem initialising MD5 Store: %+v\n", err)
	} else if verbose {
		fmt.Println("Size of new MD5 store: ", len(md5Store.Data))
	}

	docs = FindAcceptablePaths(bitsavers_index_filename)

	// We want to produce a map of unique documents.
	// If an MD5 is present, that's enough to guarantee uniqueness.
	// If no MD5 is present, use the part number
	// If no part number is present, use the title
	// Look for duplicate (non-empty) MD5 values

	documentsMap := MakeDocumentsFromPaths(bitsavers_md5_filename, docs, md5Store, verbose)

	// Construct the YAML data and write it out to a file
	data, err := yaml.Marshal(&documentsMap)
	if err != nil {
		log.Fatal(err)
	}

	err = os.WriteFile("bin/"+output_file, data, 0644)
	if err != nil {
		log.Fatal(err)
	}
}

// Read the bitsavers IndexByDate.txt file and build a set of paths under DEC-related diectories
// that correspond to files with acceptable file types.
// This is so that files that are unlikely to be documents can be filtered out,
// for example file types such as JPG, BIN and so on are not likely to be
// worth recording in a list of documents.

func FindAcceptablePaths(filename string) []string {
	dec_prefixes := []string{"dec/", "able/", "dilog/", "emulex/", "mentec/", "terak/"}
	reject_file_types := []string{".bin", ".gz", ".hex", ".jpg", ".lbl", ".lst", ".mcr", ".p75", ".png", ".pt", ".tar", ".tif", ".tiff", ".zip", ".dat", ".sav", ".jp2"}
	accept_file_types := []string{".html", ".pdf", ".txt", ".doc", ".ln03"}

	// Open the bitsavers index file, complaining loudly on failure
	file, err := os.Open(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	// The index file has lines of the form:
	//      2021-09-24 22:05:17 dg/software/diag/085-000099-00_cs30-dtos-rev-00-00-update-00.pdf
	// the first field is a date is ISO format and the second field is a time
	// the third field is a relative file path.
	// The first component in that path represents a manufacturer. Only a few manufacturers
	// (listed in dec_prefixes) are of interest here.

	// Build an array of relevant paths.
	// Include only those with an acceptable prefix.
	// Of those, reject any with an undesirable suffix (e.g. ".jpg").

	var docs []string

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var parts []string
		var path string
		parts = strings.Fields(scanner.Text())
		path = parts[2]
		for _, prefix := range dec_prefixes {
			if strings.HasPrefix(path, prefix) {
				// Here if the path has a desired prefix
				fileType := filepath.Ext(path)
				if contains(reject_file_types, strings.ToLower(fileType)) {
					// This file type should be rejected
					break
				} else if contains(accept_file_types, strings.ToLower(fileType)) {
					// This type is acceptable, so carry on
				} else {
					// The current file type is neither explicitly rejected not accepted.
					// Complain bitterly in the hope that this omission will be fixed.
					// The file type is accepted, for now.
					fmt.Printf("File type [%s] encountered that is in neither the REJECT nor the ACCEPT list\n", fileType)
				}
				// At this point path is a non-empty string if it has a desired manufacturer and does NOT have an undesired file type
				if len(path) > 0 {
					docs = append(docs, path)
				}
				break
			}
		}
	}

	// Stop now if any error occurred.
	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}

	sort.Strings(docs)

	return docs
}

// This function checks if a slice contains a specified string.
// Go 1.21 provides this functionality, but this code is being developed under Go 1.20.
func contains(s []string, candidate string) bool {
	// Simply check
	for _, v := range s {
		if v == candidate {
			return true
		}
	}
	return false
}

// This function function creates a Document struct with some default values set
func CreateBitsaversDocument(path string) Document {
	var newDocument Document
	newDocument.Md5 = ""
	newDocument.PubDate = ""
	newDocument.PdfCreator = ""
	newDocument.PdfProducer = ""
	newDocument.PdfVersion = ""
	newDocument.PdfModified = ""
	newDocument.Collection = "bitsavers"
	newDocument.Size = 0
	newDocument.Filepath = bitsavers_prefix + path

	return newDocument
}

// Given a list of file paths for documents on bitsavers, this function
// analyses each path and turns it into a Document struct.
//
// If the file path appears in the available MD5 data file, then that MD5 is used in the Document.
func MakeDocumentsFromPaths(md5File string, documentPaths []string, md5Store *persistentstore.Store[string, string], verbose bool) map[string]Document {
	documentsMap := make(map[string]Document)
	for _, path := range documentPaths {
		if strings.HasPrefix(path, "dec/pdp11/microfiche/Diagnostic_Program_Listings/") || strings.HasPrefix(path, "dec/vax/microfiche/vms-source-listings/") {
			continue
		}

		newDocument := CreateBitsaversDocument(path)
		filename := filepath.Base(path)

		fileType := strings.ToUpper(filepath.Ext(path))
		newDocument.Format = fileType[1:] // filepath.Ext returns a string that starts with a "." but that's not wanted in the Document.Format field

		// Remove the file type from the filename to leave something that makes up a provisional title
		filename = filename[:len(filename)-len(fileType)]

		// The part number is the first part of the filename, up to the first underscore ("_"), if any.
		// The title is everything apart from the part number. If there is no part number then everything is the title.
		part_num, title, part_num_found := strings.Cut(filename, "_")
		if part_num_found {
			newDocument.PartNum = part_num
			newDocument.Title = title
		} else {
			// If no "_" found, there is no part number and the whole filename is the title
			newDocument.PartNum = ""
			newDocument.Title = filename
		}

		// If the title ends with a three letter month abbreviation (the first letter capitalised) and a plausible two digit year, then pull that out as a publication date.
		var monthNames = map[string]string{"Jan": "01", "Feb": "02", "Mar": "03", "Apr": "04", "May": "05", "Jun": "06", "Jul": "07", "Aug": "08", "Sep": "09", "Oct": "10", "Nov": "11", "Dec": "12"}

		titleLength := len(newDocument.Title)

		if titleLength > 7 {
			if string(newDocument.Title[titleLength-6]) == "_" {
				possibleMonth := newDocument.Title[titleLength-5 : titleLength-2]
				possibleYear := newDocument.Title[titleLength-2 : titleLength]
				if monthNumber, ok := monthNames[possibleMonth]; ok {
					newDocument.Title = newDocument.Title[0 : titleLength-6]
					newDocument.PubDate = "19" + possibleYear + "-" + monthNumber
					// fmt.Printf("DATE SEEN:  DATE:[%10s] TL:[%s] %d %s\n", newDocument.PubDate, newDocument.Title, titleLength, possibleMonth)
				} else {
					if verbose {
						fmt.Printf("NO DATE:    DATE:[%10s] TL:[%s] M:[%s]\n", newDocument.PubDate, newDocument.Title, possibleMonth)
					}
				}
			} else {
				// fmt.Printf("No procesing: saw [%s] in [%s] %d\n", string(newDocument.Title[titleLength-6]), newDocument.Title, titleLength)
			}
		}

		lookup_key := bitsavers_prefix + path
		md5_store_found := false
		md5_store_checksum := ""
		if md5, found := md5Store.Lookup(lookup_key); found {
			if verbose {
				fmt.Printf("MD5 Store: Found %s for %s\n", md5, filename)
			}
			md5_store_checksum = md5
			md5_store_found = true
		}

		key := "bitsavers@" + path
		if md5_store_found {
			newDocument.Md5 = md5_store_checksum
			key = md5_store_checksum
			newDocument.Md5 = md5_store_checksum
		} else {
			newDocument.Md5 = "XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"
			if part_num_found {
				newDocument.Md5 = "PART: " + newDocument.PartNum
			} else {
				newDocument.Md5 = "TITLE: " + newDocument.Title
			}
			fmt.Println("entry without MD5:    ", path)
			if md5_store_found {
				fmt.Printf("Found in new store but not old: %s\n", path)
			}
		}

		documentsMap[key] = newDocument
	}
	return documentsMap
}
