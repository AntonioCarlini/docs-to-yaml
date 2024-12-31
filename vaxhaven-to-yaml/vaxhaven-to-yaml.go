package main

import (
	"docs-to-yaml/internal/document"
	"docs-to-yaml/internal/persistentstore"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
)

// This program takes the a subset of VaxHaven documentation index pages and the set of known VaxHaven documents
// and updates the latter with new information from the former.
// As a side effect the File Size Store may be updated with new values.

type Document = document.Document

type Store = persistentstore.Store[string, int64]

var vaxhaven_prefix = "http://www.vaxhaven.com"

func main() {

	vaxhaven_data := "data/VaxHaven.txt"
	output_file := "bin/vaxhaven.yaml"
	fileSizeStoreFilename := "bin/filesize.store"
	fileSizeStoreCreate := true
	verbose := false

	fileSizeStoreInstantiation := persistentstore.Store[string, int64]{}
	fileSizeStore, err := fileSizeStoreInstantiation.Init(fileSizeStoreFilename, fileSizeStoreCreate, verbose)
	if err != nil {
		fmt.Printf("Problem initialising FileSize Store: %+v\n", err)
	} else if !verbose {
		fmt.Println("Size of new FileSize store: ", len(fileSizeStore.Data))
	}

	documentsMap := ParseNewData(vaxhaven_data, fileSizeStore, verbose)

	// Construct the YAML data and write it out to a file
	data, err := yaml.Marshal(&documentsMap)
	if err != nil {
		log.Fatal(err)
	}

	err = os.WriteFile(output_file, data, 0644)
	if err != nil {
		log.Fatal(err)
	}

	// If the FileSize Store is active and it has been modified ... save it
	fileSizeStore.Save(fileSizeStoreFilename)

}

// This function parses the VaxHaven HTML that indexes the documents and produces a set of
// corresponding YAML data. The input HTML should be a concatenation of the individual VaxHaven
// documentation index pages.
func ParseNewData(filename string, fileSizeStore *Store, verbose bool) map[string]Document {

	// Open the bitsavers index file, complaining loudly on failure
	file, err := os.ReadFile(filename)
	if err != nil {
		log.Fatal(err)
	}

	// The interesting data looks like this:
	// <tr>
	// <td><a href="/images/d/dd/AA-0196C-TK.pdf" class="internal" title="AA-0196C-TK.pdf">AA-0196C-TK</a></td>
	// <td>DECsystem10/20 ALGOL Programmer's Guide</td>
	// <td>1977 April
	// </td></tr>
	//
	// So the part number, document title, date and path are all available

	documentsMap := make(map[string]Document)

	r_rows := regexp.MustCompile(`(?ms)<tr>(.*?)</tr>`)
	r_data := regexp.MustCompile(`(?ms)<td>\s*<a\s+href="(.*?)".*?>(.*?)</a></td>.*?<td>(.*?)</td>.*?<td>(.*?)</td>`)
	r_data_no_date := regexp.MustCompile(`(?ms)<td>\s*<a\s+href="(.*?)".*?>(.*?)</a></td>.*?<td>(.*?)</td>`)
	fmt.Println(r_rows)
	rows := r_rows.FindAllStringSubmatch(string(file), -1)
	fmt.Println("Found ", len(rows), "matches")
	for i, match := range rows {
		data := r_data.FindAllStringSubmatch(match[1], -1)
		// data[0][1] => path (href)
		// data[0][2] => part num
		// data[0][3] => title
		// data[0][4] => date
		docDate := ""
		if data == nil {
			data = r_data_no_date.FindAllStringSubmatch(match[1], -1)
			if data == nil {
				fmt.Println("No match at all for [", match[1], "] #", i)
				continue
			}
		} else {
			docDate = data[0][4]
		}
		// fmt.Println("data size = ", len(data), " => ", len(data[0]), "=>", data[0][1], "=> ", data[0][2], " => ", data[0][3], data[0][4])
		document := CreateVaxHavenDocument(vaxhaven_prefix + data[0][1])
		document.PartNum = strings.TrimSpace(data[0][2])
		document.Title = strings.TrimSuffix(strings.TrimSpace(data[0][3]), "\n")
		if len(data[0]) >= 4 {
			document.PubDate = ConvertVaxHavenDate(docDate)
			if document.PubDate == "XXXX" {
				fmt.Printf("Suspicious date [%s] for %s (%s)\n", docDate, document.Title, document.Filepath)
			}
		}

		fileSize, err := CalculatefileSize(document.Filepath, fileSizeStore, verbose)
		if err != nil {
			log.Fatal(err)
		}
		document.Size = fileSize
		// fmt.Println("document: ", document)

		if _, found := documentsMap[document.PartNum]; found {
			fmt.Printf("VaxHaven docuemnt repeated: Found [%s, %s] repeated as %s\n", document.PartNum, documentsMap[document.PartNum].Filepath, document.Filepath)
		} else {
			documentsMap[document.PartNum] = document
		}

	}
	fmt.Println("Number of docs found: ", len(documentsMap))
	return documentsMap
}

// This function function creates a Document struct with some default values set.
func CreateVaxHavenDocument(path string) Document {
	var newDocument Document
	newDocument.Md5 = ""
	newDocument.PubDate = ""
	newDocument.PdfCreator = ""
	newDocument.PdfProducer = ""
	newDocument.PdfVersion = ""
	newDocument.PdfModified = ""
	newDocument.Collection = "VaxHaven"
	newDocument.Size = 0
	newDocument.Filepath = path

	return newDocument
}

// Takes a VAXHaven date, in the format "YYYY Month", where "Month"
// is specified in full, and converts intoa string of the form "YYYY-MM".
func ConvertVaxHavenDate(date string) string {
	if date == "" {
		return ""
	}
	if len(date) < 4 {
		// fmt.Println("TO FIX: Suspicious date: [", date, "]")
		return "XXXX"
	}
	year := date[0:4]
	month := strings.ToLower(strings.TrimSpace(date[5:]))
	result := "YYYY-MM"
	if len(month) < 1 {
		result = year
	} else {
		// If the title ends with a three letter month abbreviation (the first letter capitalised) and a plausible two digit year, then pull that out as a publication date.
		var monthNames = map[string]string{"january": "01", "february": "02", "march": "03", "april": "04", "may": "05", "june": "06", "july": "07", "august": "08", "september": "09", "october": "10", "november": "11", "december": "12"}
		if monthNumber, ok := monthNames[month]; ok {
			result = year + "-" + monthNumber
		} else {
			log.Fatalf("Bad date: [%s] year=[%s] month=[%s]", date, year, month)
		}
	}
	return result
}

// Return the fileSize for the specified file.
// Start by looking up the filename (path) in the store and return a pre-computed fileSize sum if found.
// Otherwise, compute the fileSize sum, add the entry to the store and return the computed fileSize sum.
var tempCount int = 0

func CalculatefileSize(filename string, fileSizeStore *Store, verbose bool) (int64, error) {

	// Lookup the filename (path) in the store; if found report that as the fileSize sum
	if fileSize, found := fileSizeStore.Lookup(filename); found {
		if verbose {
			fmt.Printf("fileSize Store: Found %d for %s\n", fileSize, filename)
		}
		return fileSize, nil
	}
	tempCount += 1
	if tempCount > 10000 {
		fmt.Println("Too many URL lookups")
		return 0, nil
	}

	// The filename (path) is not in the store.
	// Ask for the remote file size
	url := filename
	resp, err := http.Head(url)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	time.Sleep(2 * time.Second)
	fileSize, _ := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	fmt.Printf("fileSize Store: saved %d for %s\n", fileSize, filename)
	fileSizeStore.Update(filename, fileSize)

	return fileSize, nil
}
