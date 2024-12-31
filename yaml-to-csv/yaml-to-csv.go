package main

import (
	"docs-to-yaml/internal/document"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"

	"gopkg.in/yaml.v2"
)

//
// This program reads in one or more YAML files, each describing a set of documents, and outputs a CSV file describing much of the same information in a more concise form.
//
// The reason for generating this CSV is that it is easier to grep and find a meaningful result when hunting for a specific document, where the title or part number is known.
//

type Document = document.Document

// Main entry point.
// Processes a set of YAML files, each of which contains details about a set of Document records
// For each Document, one CSV record is created.
// Finally the accumulated CSV records are written to the specified CSV file.
//
// No deduplication or other validation or processing is performed.
//
// To run the program:
//   go run yaml-to-csv/yaml-to-csv.go yaml-file(s) --verbose --csv output-csv-file  YAML-FILE-1 [, YAML-FILE-2 [, ...]]

func main() {
	verbose := flag.Bool("verbose", false, "Enable verbose reporting")
	csvOutputFilename := flag.String("csv", "", "filepath of the output file to hold the generated CSV")

	flag.Parse()

	if *csvOutputFilename == "" {
		log.Fatal("Please supply a filespec for the output CSV")
	}

	var csvDocs [][]string

	for _, yaml_file := range flag.Args() {
		documentsMap := make(map[string]Document)

		if *verbose {
			fmt.Printf("Processing YAML file: [%s]\n", yaml_file)
		}
		yaml_text, err := os.ReadFile(yaml_file)
		if err != nil {
			log.Printf("yamlFile read err for %s,  #%v ", yaml_file, err)
		}
		err = yaml.Unmarshal(yaml_text, &documentsMap)
		if err != nil {
			log.Fatalf("Unmarshal error for %s: %v", yaml_file, err)
		}

		for _, doc := range documentsMap {
			csvDocs = append(csvDocs, ConvertDocumentToCsv(doc))
		}

		if *verbose {
			fmt.Printf("Finished procesing YAML %s, having found %d docs, for a total of %d CSV records\n", yaml_file, len(documentsMap), len(csvDocs))
		}
	}
	fmt.Printf("Found %d records in total\n", len(csvDocs))

	csvFile, err := os.Create(*csvOutputFilename)

	if err != nil {
		log.Fatalf("CSV file open failed for %s, %v\n", *csvOutputFilename, err)
	}
	defer csvFile.Close()

	csvWriter := csv.NewWriter(csvFile)
	defer csvWriter.Flush()

	header := []string{"Record", "Title", "File", "URL", "Date", "Part Number", "Options"}
	err = csvWriter.Write(header)
	if err != nil {
		fmt.Println("Error writing header to CSV:", err)
	}

	for _, rec := range csvDocs {
		err = csvWriter.Write(rec)
		if err != nil {
			fmt.Println("Error writing record to CSV:", err)
		}
	}
}

// This table shows the fields in a CSV record and the Document members from which each CSV field is derived.
//
// | Field #  | Contents             | CSV field
// |----------|----------------------|----------------
// |       1  | _ Record type_       | "Doc" (fixed text)
// |       2  | _Document title_     | .Title
// |       3  | _Local file path_    | .Filepath
// |       4  | _Original URL_       | .PublicUrl
// |       5  | _Document date_      | .PubDate
// |       6  | _Part number_        | .PartNum
// |       7  | _Options_            |
//
// The CSV 'options' field contains the following sub-options:
//
//	md5='' doc.Md5
//	collection='' not currently available
func ConvertDocumentToCsv(doc Document) []string {
	options := fmt.Sprintf("'md5=%s'", doc.Md5)
	return []string{
		"Doc",
		doc.Title,
		doc.Filepath,
		doc.PublicUrl,
		doc.PubDate,
		doc.PartNum,
		options,
	}
}
