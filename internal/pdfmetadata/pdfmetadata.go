package pdfmetadata

import (
	"fmt"
	"log"
	"strings"

	"github.com/barasher/go-exiftool"
)

// The PdfMetdata struct is used to record a subset of metadata that can be extracted from a PDF file
type PdfMetadata struct {
	Creator  string
	Producer string
	Format   string
	Modified string
}

// Given a PDF file, this function finds the associated metdata and returns those elements that will be stored in the YAML.
func ExtractPdfMetadata(pdfFilename string) PdfMetadata {
	et, err := exiftool.NewExiftool()
	if err != nil {
		log.Printf("Error when intializing: %v\n", err)
	}
	defer et.Close()

	fileInfos := et.ExtractMetadata(pdfFilename)
	metadata := PdfMetadata{}
	for _, fileInfo := range fileInfos {
		if fileInfo.Err != nil {
			fmt.Printf("Error concerning %v: %v\n", fileInfo.File, fileInfo.Err)
			continue
		}

		for k, v := range fileInfo.Fields {
			if k == "Creator" {
				metadata.Creator = v.(string)
			}
			if k == "Producer" {
				metadata.Producer = v.(string)
			}
			if k == "PDFVersion" {
				metadata.Format = strings.TrimRight(fmt.Sprintf("%f", v.(float64)), "0")
			}
			if k == "ModifyDate" {
				metadata.Modified = v.(string)
			}
		}
	}

	return metadata
}
