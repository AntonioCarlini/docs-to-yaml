package pdfmetadata

import (
	"fmt"
	"io/fs"
	"log"
	"os"
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
		log.Printf("Error when initializing: %v\n", err)
	}
	defer et.Close()

	fileInfos := et.ExtractMetadata(pdfFilename)
	return parseExifOutput(fileInfos) // Use the new helper logic
}

func ZExtractPdfMetadata(pdfFilename string) PdfMetadata {
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

func ExtractPdfMetadataFromFS(fsys fs.FS, pdfPath string) PdfMetadata {
	// 1. Read data from the ISO into memory
	data, err := fs.ReadFile(fsys, pdfPath)
	if err != nil {
		log.Printf("Error reading from FS: %v\n", err)
		return PdfMetadata{}
	}

	// 2. Create a temporary file on the local disk
	tmpFile, err := os.CreateTemp("", "exiftool-*.pdf")
	if err != nil {
		log.Printf("Error creating temp file: %v\n", err)
		return PdfMetadata{}
	}
	defer os.Remove(tmpFile.Name()) // Clean up after we are done

	// 3. Write the bytes to the temp file
	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return PdfMetadata{}
	}
	tmpFile.Close() // Close so Exiftool can access it

	// 4. Use the standard ExtractMetadata call on the temp path
	et, err := exiftool.NewExiftool()
	if err != nil {
		log.Printf("Error when initializing: %v\n", err)
	}
	defer et.Close()

	fileInfos := et.ExtractMetadata(tmpFile.Name())
	return parseExifOutput(fileInfos)
}

// Private helper to avoid repetition
func parseExifOutput(fileInfos []exiftool.FileMetadata) PdfMetadata {
	metadata := PdfMetadata{}
	for _, fileInfo := range fileInfos {
		if fileInfo.Err != nil {
			fmt.Printf("Error concerning %v: %v\n", fileInfo.File, fileInfo.Err)
			continue
		}

		for k, v := range fileInfo.Fields {
			switch k {
			case "Creator":
				metadata.Creator = v.(string)
			case "Producer":
				metadata.Producer = v.(string)
			case "PDFVersion":
				metadata.Format = strings.TrimRight(fmt.Sprintf("%f", v.(float64)), "0")
			case "ModifyDate":
				metadata.Modified = v.(string)
			}
		}
	}
	return metadata
}
