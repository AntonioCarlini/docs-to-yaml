package document

import (
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"regexp"
	"strings"
)

// The Document struct is how per-electronic-document data is represented in YAML
type Document struct {
	Format      string // File format (PDF, TXT, etc.)
	Size        int64  // File size in bytes
	Md5         string // File MD5 checksum
	Title       string // Document title
	PubDate     string // The publication date
	PartNum     string // The manufacturer identifier or part number for the document
	PdfCreator  string // PDF data: "Creator"
	PdfProducer string // PDF data: "Producer"
	PdfVersion  string // PDF data: "Format", this will be, for example, "PDF-1.2"
	PdfModified string // PDF data: "Modified"
	Collection  string // Name of collection this document is found in (e.g. "bitsavers")
	Filepath    string // relative file path of document in collection
}

// Determine the file format. This will be TXT, PDF, RNO etc.

// For now, it can just be the filetype, as long as it is one of
// a recognised set. If necessary this could be expanded to use the mimetype
// package.
// Note that "HTM" will be returned as "HTML": both types exist in the collection but it makes no sense to allow both!
// Similarly "JPG" will be returned as "JPEG".
var KnownFileTypes = [...]string{"PDF", "TXT", "MEM", "RNO", "PS", "HTM", "HTML", "ZIP", "LN3", "TIF", "JPG", "JPEG"}

// Sometimes the same file structure may be indicated by multiple filetypes, for
// example HTML files may be ".HTM" or ".HTML", the JPEG file format might be ".JPEG" or ".JPG"
// and TIF files may be ".TIF" or ".TIFF".
//
// This function produces a consistent format string for any known type and returns "???"
// and an error for an unrecognised file type.

var FileTypesToRecategorise = map[string]string{"HTM": "HTML", "JP2": "JPEG", "JPG": "JPEG", "TIF": "TIFF"}

func DetermineDocumentFormat(filename string) (string, error) {
	filetype := strings.TrimPrefix(strings.ToUpper(filepath.Ext(filename)), ".")
	if ftype, found := FileTypesToRecategorise[filetype]; found {
		filetype = ftype
	}

	for _, entry := range KnownFileTypes {
		if entry == filetype {
			return filetype, nil
		}
	}
	// log.Fatalf("Unknown filetype: %s for filename %s\n", filetype, filename) // TODO

	return "???", errors.New("Unknown file type when trying to determine docuemnt format")
}

// Attempt to parse the document filename to produce a part number, a title and a publication date.
// This assumes that the title roughly follows the current bitsavers format of:
// DEC-PART-NUM_A_title_of_many_parts_Jan00.pdf
// So everything up to the first underscore is considered a possible part number.
// Everything after the last underscore (but excluding the filetype) is a potential date.
// The rest is a title with underscore taking the place of any spaces.

// TODO
func DetermineDocumentPropertiesFromPath(path string, verbose bool) Document {
	var doc Document
	doc.PartNum = "MADE-UP-DATE"
	doc.Title = "*** Invented Title ***"
	doc.PubDate = "1758-11-04"
	//fmt.Println("doc properties with called with [", path, "] len=", len(path))
	fmt.Println("****************************************** This function is currently a non-functional placeholder function ******************************************")

	filename := filepath.Base(path)
	fileType := strings.ToUpper(filepath.Ext(path))
	format, err := DetermineDocumentFormat(path)
	if err != nil {
		//.Fatalf("failed to find format in %s, err=%s\n", path, err)
		fmt.Printf("Fatal error avoided for [%s]\n", path)
	}
	doc.Format = format

	// Remove the file type from the filename to leave something that makes up a provisional title
	filename = filename[:len(filename)-len(fileType)]

	// The part number is the first part of the filename, up to the first underscore ("_"), if any.
	// The title is everything apart from the part number. If there is no part number then everything is the title.
	potentialPartNum, _ /*title*/, partNumFound := strings.Cut(filename, "_")
	if partNumFound {
		partNumFound = ValidateDecPartNumber(potentialPartNum)
	}
	if partNumFound {

		// If no "_" found, there is no part number and the whole filename is the title
		// newDocument.PartNum = ""
		// newDocument.Title = filename
	} else {
		fmt.Printf("Bad Part #: [%s] in %s\n", potentialPartNum, path)
	}

	// If the title ends with a three letter month abbreviation (the first letter capitalised) and a plausible two digit year, then pull that out as a publication date.
	var monthNames = map[string]string{"Jan": "01", "Feb": "02", "Mar": "03", "Apr": "04", "May": "05", "Jun": "06", "Jul": "07", "Aug": "08", "Sep": "09", "Oct": "10", "Nov": "11", "Dec": "12"}

	titleLength := len(doc.Title)

	if titleLength > 7 {
		if string(doc.Title[titleLength-6]) == "_" {
			possibleMonth := doc.Title[titleLength-5 : titleLength-2]
			possibleYear := doc.Title[titleLength-2 : titleLength]
			if monthNumber, ok := monthNames[possibleMonth]; ok {
				doc.Title = doc.Title[0 : titleLength-6]
				doc.PubDate = "19" + possibleYear + "-" + monthNumber
				// fmt.Printf("DATE SEEN:  DATE:[%10s] TL:[%s] %d %s\n", newDocument.PubDate, newDocument.Title, titleLength, possibleMonth)
			} else {
				if verbose {
					fmt.Printf("NO DATE:    DATE:[%10s] TL:[%s] M:[%s]\n", doc.PubDate, doc.Title, possibleMonth)
				}
			}
		} else {
			// fmt.Printf("No procesing: saw [%s] in [%s] %d\n", string(newDocument.Title[titleLength-6]), newDocument.Title, titleLength)
		}
	}

	return doc
}

// Checks if the string supplied looks like a known DEC part number format.
//
// Allow the following part number formats (where lowercase means any alphanumeric character and uppercase means a fixed value):
//
//	aa-aaaaa-aa.ccc
//	DEC-11-abcde-b-d
//	K-MN-abcdef-aa-abcd.abc
func ValidateDecPartNumber(partNumber string) bool {
	pn := strings.ToUpper(partNumber)
	match, err := regexp.MatchString(`^[[:alnum:]]{2}-[[:alnum:]]{4,5}(-|\.)[[:alnum:]]{2}((-|.)[[:alnum:]]{2,4})?$`, pn)
	if err != nil {
		log.Fatal("EK-NNNNN-JJ regexp faulty")
	}
	if match {
		return true
	}

	match, err = regexp.MatchString(`^DEC-11-[[:alnum:]]{5}-[[:alnum:]]-[[:alnum:]]$`, pn)
	if err != nil {
		log.Fatal("DEC-11-AAAAA-B-D regexp faulty")
	}
	if match {
		return true
	}

	match, err = regexp.MatchString(`^K-MN-[[:alnum:]]{6}-[[:alnum:]]{2}-[[:alnum:]]{4}(-|.)[[:alnum:]]{3}$`, pn)
	if err != nil {
		log.Fatal("K-MN-AS8X00-00-JG00.A06 regexp faulty")
	}
	if match {
		return true
	}

	// Nothing so far has matched, so assume this is not a DEC part number
	return false
}
