package main

// This program works through a tree of files, starting at a specified root,
// and produces a YAML file that describes the contents.
//
// The intention is to do the bulk of the work necessary to produce a YAML
// file that describes each document's part number, title, date, MD5 checksum, etc.

// This is currently a work in progress.

// TODO:
// If CSV specified:
//   read CSV
//   match files by MD5
//   report any failed matches
//   report any title changes
//   force replacement yaml (same name but .new.yaml)

import (
	"crypto/md5"
	"docs-to-yaml/internal/document"
	"docs-to-yaml/internal/pdfmetadata"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

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
	update := flag.Bool("update", false, "Enable verbose reporting")

	flag.Parse()

	var err error

	if *yamlOutputFilename == "" {
		log.Fatal("Please supply a filespec for the output YAML")
	}

	var mapByMd5 map[string]Document = make(map[string]Document)
	var mapByFilepath map[string]Document = make(map[string]Document)
	var csvMapByMd5 map[string]Document = make(map[string]Document)

	if *update {
		fmt.Println("Update specified: loading CSV")
		/* TODO read CSV file into Document objects*/
		csvMapByMd5, err = LoadCSV(*treeRoot)
		if err != nil {
			log.Fatalf("impossible to process CSV: %s", err)
		}
	} else {
		fmt.Println("CSV NOT specified")
	}

	var yamlSource = *yamlOutputFilename

	if *update {
		yamlSource = *treeRoot
		if (*treeRoot)[len(*treeRoot)-1:] != "/" {
			yamlSource += "/"
		}
		yamlSource += "index.yaml"
	}

	// TODO:
	// --update means produce updated YAML from index.yaml and index.csv (at least one must exist)
	//

	// Start by reading the output yaml file.
	fmt.Printf("Seeding YAML with %s\n", yamlSource)
	initialData, err := YamlDataInit(yamlSource)
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
	var relativePaths []string
	err = filepath.WalkDir(*treeRoot, func(path string, d fs.DirEntry, err error) error {
		if !d.IsDir() {
			relativePaths = append(relativePaths, path[treePrefixLength:])
		}
		return nil
	})
	if err != nil {
		log.Fatalf("impossible to walk directories: %s", err)
	}

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
			delete(mapByMd5, v.Filepath) // Eliminate the matching MD5 entry too
		} else {
			mapByFilepath[v.Filepath] = v
		}
	}

	if len(mapByMd5) != len(mapByFilepath) {
		log.Fatalf("After all processing, MD5 and Filepath maps are different sizes; %d docs listed by MD5 and %d listed by filepath\n", len(mapByMd5), len(mapByFilepath))
	} else {
		fmt.Printf("After loading and processing YAML file, %d documents are known (by filepath and by MD5).\n", len(mapByFilepath))
	}

	for _, relativeFilepath := range relativePaths {
		// Some 'index' files are added to a local file tree for tracking and cataloguing purposes.
		// These are not part of the original data set and should not be recorded as a Document.
		if (relativeFilepath == "index.csv") || (relativeFilepath == "index.yaml") || (relativeFilepath == "index.pdf") || (relativeFilepath == "index.txt") || (relativeFilepath == "index.html") {
			continue
		}

		doc, found := mapByFilepath[relativeFilepath]
		if !found {
			doc = CreateLocalDocument(relativeFilepath)
		}
		originalMd5 := doc.Md5

		// Set up properties that are determined by the filepath, but only if they are currently missing
		data := document.DetermineDocumentPropertiesFromPath(doc.Filepath, *verbose)
		if doc.Format == "" {
			doc.Format = data.Format
		}
		if doc.Title == "" {
			doc.Title = data.Title
			document.SetFlags(&doc, "T")
		}
		if doc.PartNum == "" {
			doc.PartNum = data.PartNum
			document.SetFlags(&doc, "P")
		}
		if doc.PubDate == "" {
			doc.PubDate = data.PubDate
			document.SetFlags(&doc, "D")
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
				doc.Md5 = md5Checksum
			}
		}

		md5Key := document.BuildKeyFromDocument(doc)

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
		mapByFilepath[relativeFilepath] = doc
		// MD5 checksum may have changed: if so, remove the old entry from the map keyed on MD5 checksum
		if originalMd5 != md5Key {
			delete(mapByMd5, originalMd5)
		}
		mapByMd5[md5Key] = doc
		if *verbose {
			fmt.Printf("Added MD5 map entry key=%s title=%s\n", md5Key, doc.Title)
		}

		pathOK, badChar, nonAsciiChar := CheckPathForInadvisableCharacters(doc.Filepath)
		if !pathOK {
			fmt.Printf("Reconsider path %s,", doc.Filepath)
			if badChar != "" {
				fmt.Printf(" contains [%s]", badChar)
			}
			if nonAsciiChar != "" {
				fmt.Printf(" contains non-ASCII [%s]", nonAsciiChar)
			}
			fmt.Println()
		}
	}

	// If MD5 checksums have been generated, then there should be no blank MD5 checksums and there
	// should be no documents where the MD5 checksum matches the filepath (at least if we ignore the pathological case
	// of a document that is named for its MD5 checksum!).
	// Eliminate any document in the map-by-MD5 that meets either of these criteria.

	if *md5Gen {
		for k, v := range mapByMd5 {
			if (v.Md5 == "") || (v.Md5 == v.Filepath) {
				fmt.Printf("Eliminating MD5 entry for path %s using key [%s]\n", v.Filepath, k)
				delete(mapByMd5, k)
			}
		}
	}

	// Ensure that each document is listed
	fmt.Println("Finished with this many documents by filepath: ", len(mapByFilepath), " and this many by MD5: ", len(mapByMd5))

	if *fnfList || *fnfDiscard {
		for k, d := range mapByFilepath {
			fullPath := treePrefix + d.Filepath
			if d.Filepath == "" && (k != d.Md5) {
				fullPath = treePrefix + k
			}
			if *verbose {
				fmt.Println("checking ", fullPath)
			}
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

	// Loop through docs in CSV
	// If no key match in mapByMd5, complain
	// If key matches then some fields must match
	// If all OK, override title if different
	for k, d := range csvMapByMd5 {
		if doc, ok := mapByMd5[k]; ok {
			if (doc.Md5 != d.Md5) || (doc.Filepath != d.Filepath) {
				fmt.Printf("CSV doc %s with MD5 %s mismatched (%s in mapByMd5)\n", k, d.Md5, doc.Md5)
				continue
			}
			if doc.Filepath != d.Filepath {
				fmt.Printf("CSV doc %s with Filepath %s mismatched (%s in mapByMd5)\n", k, d.Filepath, doc.Filepath)
				continue
			}
			if (doc.PublicUrl != d.PublicUrl) && (doc.PublicUrl != "") && (d.PublicUrl != "") {
				fmt.Printf("CSV doc %s with URL %s mismatched (%s in mapByMd5)\n", k, d.PublicUrl, doc.PublicUrl)
				continue
			}
			if (doc.PubDate != d.PubDate) || (doc.PartNum != d.PartNum) {
				fmt.Printf("CSV doc %s with Date %s mismatched (%s in mapByMd5)\n", k, d.PubDate, doc.PubDate)
				continue
			}
			if doc.PartNum != d.PartNum {
				fmt.Printf("CSV doc %s with Part Num %s mismatched (%s in mapByMd5)\n", k, d.PartNum, doc.PartNum)
				continue
			}
			// Here the CSV and generated YAML agree, so update the title if necessary
			var mapEntryUpdated = false

			if doc.Title != d.Title {
				doc.Title = d.Title
				mapEntryUpdated = true
				fmt.Printf("Updated title for %s from CSV (%s)\n", doc.Md5, doc.Title)
			}
			// Update the URL if appropriate
			if (doc.PublicUrl != d.PublicUrl) && (doc.PublicUrl == "") {
				doc.PublicUrl = d.PublicUrl
				mapEntryUpdated = true
				fmt.Printf("Updated URL for %s from CSV (%s): %s\n", doc.Md5, doc.Title, doc.PublicUrl)
			}
			if mapEntryUpdated {
				mapByMd5[k] = doc
			}
		} else {
			fmt.Printf("CSV doc %s with MD5 %s not found in mapByMd5\n", k, d.Title)
		}
	}

	// After all the manipulation, there must be exactly the same number of documents in the MD5 and Filepath maps
	// (unless MD5 processing has not been enabled)
	if *md5Gen && (len(mapByMd5) != len(mapByFilepath)) {
		// log.Fatalf("After all final processing, MD5 and Filepath maps are different sizes; %d docs listed by MD5 and %d listed by filepath\n", len(mapByMd5), len(mapByFilepath))
		fmt.Printf("After all final processing, MD5 and Filepath maps are different sizes; %d docs listed by MD5 and %d listed by filepath\n", len(mapByMd5), len(mapByFilepath))
		// List all docs that are in MD5 but not in filepath
		/*		for _, d := range mapByMd5 {
				if mapByFilepath[d.Filepath]
						}
		*/ // List all docs that are in filepath but not in MD5
	}

	// Write the output YAML file
	if *verbose {
		fmt.Printf("Saving %d documents\n", len(mapByMd5))
	}
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

// This function reads a CSV file and unpacks the information into a map of Document objects
func LoadCSV(filepath string) (map[string]Document, error) {
	var docs map[string]Document = make(map[string]Document)

	var csvFilepath = filepath
	if filepath[len(filepath)-1:] != "/" {
		csvFilepath += "/"
	}
	csvFilepath += "index.csv"
	csvFile, err := os.Open(csvFilepath)
	if err != nil {
		return nil, err
	}
	defer csvFile.Close()
	reader := csv.NewReader(csvFile)
	csvRecords, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	for _, row := range csvRecords {
		// Ignore any records that do not relate to a specific document
		if row[0] != "Doc" {
			continue
		}
		newDoc := CreateLocalDocument(row[2])
		newDoc.Title = row[1]
		newDoc.PublicUrl = row[3]
		newDoc.PubDate = row[4]
		newDoc.PartNum = row[5]
		newDoc.Md5 = row[6]
		// TODO handle collection in options?
		docKey := document.BuildKeyFromDocument(newDoc)
		fmt.Printf("CSV doc MD5=[%s] Key=[%s]\n", newDoc.Md5, docKey)
		docs[docKey] = newDoc
	}

	return docs, nil
}

// This function function creates a Document struct with some default values set
func CreateLocalDocument(relativeFilepath string) Document {
	var newDocument Document
	newDocument.Md5 = ""
	newDocument.PubDate = ""
	newDocument.PdfCreator = ""
	newDocument.PdfProducer = ""
	newDocument.PdfVersion = ""
	newDocument.PdfModified = ""
	newDocument.Collection = "local-pending"
	newDocument.Size = 0
	newDocument.Filepath = relativeFilepath

	return newDocument
}

// Look for unfortunate characters in a filepath.
//
// Note that the caller should specify the path *within* the collection, as
// that is all that will appear when the collection is copied elsewhere or
// written to optical media.
func CheckPathForInadvisableCharacters(filepath string) (bool, string, string) {
	charactersToAvoid := "#%&{}\\<>*?!$'\":@`="
	includedInadvisableCharacters := ""
	includedNonAsciiCharacters := ""
	for _, candidate := range filepath {
		if !isASCII(byte(candidate)) {
			includedNonAsciiCharacters += string(candidate)
		} else {
			if strings.Contains(charactersToAvoid, string(candidate)) {
				includedInadvisableCharacters += string(candidate)
			}
		}
	}

	return ((includedInadvisableCharacters == "") && (includedNonAsciiCharacters == "")), includedInadvisableCharacters, includedNonAsciiCharacters
}

func isASCII(character byte) bool {
	ascii := int(character)
	return (ascii < 128)
}
