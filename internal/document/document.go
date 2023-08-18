package document

import (
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"regexp"
	"strconv"
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
//
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

// Attempt to parse the document filename to produce a part number, a title, a publication date and fill in the document format.
// This assumes that the title roughly follows the current bitsavers format of:
// DEC-PART-NUM_A_title_of_many_parts_Jan00.pdf
// So everything up to the first underscore is considered a possible part number.
// Everything after the last underscore (but excluding the filetype) is a potential date.
// The rest is a title with underscore taking the place of any spaces.
// Finally the document format is decided based on the filetype.

func DetermineDocumentPropertiesFromPath(path string, verbose bool) Document {
	var doc Document
	doc.PartNum = "MADE-UP-DATE"
	doc.Title = "*** Invented Title ***"
	doc.PubDate = "1758-11-04"

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

	// Find everything before the firs underscore and validate it as a DEC part number
	partNum, title, partNumFound := strings.Cut(filename, "_")
	if partNumFound {
		partNumFound = ValidateDecPartNumber(partNum)
	}

	// If the final decision is that a valid part number has been found, record it in the Document and remove it from the title.
	// Otherwise the title (so far) is the whole original filename.
	if partNumFound {
		title = filename[len(partNum)+1:]
		doc.PartNum = partNum
	} else {
		title = filename
		fmt.Printf("Bad Part #: [%s] in %s\n", partNum, path)
	}

	// Look for a possible date. This will always be all the characters between the
	// last underscore and the end of the string (i.e. before the period of the filetype in the original filename).
	// If there is no underscore, then there is no date.
	possibleDateStart := strings.LastIndex(title, "_")
	if (possibleDateStart >= 0) && (len(title) > (possibleDateStart + 2)) {
		possibleDate := ValidateDate(title[possibleDateStart+1:])
		if possibleDate != "" {
			title = title[0:possibleDateStart]
			doc.PubDate = possibleDate
		}
	}

	// Remove any underscores from the title so far  to leave the final title
	doc.Title = strings.Replace(title, "_", " ", -1)

	return doc
}

// Construct a key for a given Document.
// If an MD5 checksum is present, use that.
// Otherwise use the part number, if it exists.
// If there is still no key try using the title.
// As a last resort, use the filepath.
func BuildKeyFromDocument(doc Document) string {
	// The best possible key is the MD5 checksum, so if one is present, use that.
	if doc.Md5 != "" {
		return doc.Md5
	}

	// Try, in turn, the part number, title and filepath
	if doc.PartNum != "" {
		return doc.PartNum
	} else if doc.Title != "" {
		return doc.Title
	}
	return doc.Filepath

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

// Check if the string supplied can be interpreted as a date.
// Currently only the formats seen in filenames on bitsavers are accepted.
// The following formats are accepted:
// YYYY     - four digit year
// YYYYMM   - four digit year and two digit month (with leading 0 if necessary)
// mmmYY    - Three letter English month abbreviation and two digit year; 50-99=> 1960-1999, 00-23 2000-2023

func ValidateDate(date string) string {
	dateLength := len(date)
	if dateLength < 4 {
		return ""
	}

	switch dateLength {
	case 4:
		year, err := strconv.Atoi(date)
		if err != nil {
			return ""
		}
		if (year >= 1960) && (year <= 2023) {
			return date
		} else {
			return ""
		}

	case 6:
		year, err := strconv.Atoi(date[0:3])
		if (err != nil) || (year < 1960) || (year > 2023) {
			return ""
		}
		month, err := strconv.Atoi(date[4:5])
		if (err != nil) || (month < 1) || (month > 12) {
			return ""
		}
		return date[0:3] + "-" + date[4:5]
	case 5:
		// If the title ends with a three letter month abbreviation (the first letter capitalised) and a plausible two digit year, then pull that out as a publication date.
		var monthNames = map[string]string{"JAN": "01", "FEB": "02", "MAR": "03", "APR": "04", "MAY": "05", "JUN": "06", "JUL": "07", "AUG": "08", "SEP": "09", "OCT": "10", "NOV": "11", "DEC": "12"}
		possibleMonth := strings.ToUpper(date[0:3])
		possibleYear := date[3:]
		if monthNumber, ok := monthNames[possibleMonth]; ok {
			// TODO make this allow for 2000 onwards!
			return "19" + possibleYear + "-" + monthNumber
		} else {
			return ""
		}
	}
	return ""
}
