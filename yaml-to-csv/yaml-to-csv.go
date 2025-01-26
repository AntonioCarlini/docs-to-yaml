package main

import (
	"docs-to-yaml/internal/document"
	"docs-to-yaml/internal/indexcsv"
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

	totalDocumentsMap := make(map[string]Document)

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

		for key, doc := range documentsMap {
			totalDocumentsMap[key] = doc
		}
		if *verbose {
			fmt.Printf("Finished procesing YAML %s, having found %d docs, for a total of %d CSV records\n", yaml_file, len(documentsMap), len(totalDocumentsMap))
		}
	}
	fmt.Printf("Found %d records in total\n", len(totalDocumentsMap))

	err := indexcsv.WriteDocumentsToCsv(totalDocumentsMap, *csvOutputFilename)
	if err != nil {
		fmt.Println("Error writing csv copy:", err)
	}
}
