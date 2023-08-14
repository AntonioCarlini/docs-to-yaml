package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	types "docs-to-yaml/structs"

	"gopkg.in/yaml.v2"
)

// This program takes the a subset of VaxHaven documentation index pages and the set of known VaxHaven documents
// and updates the latter with new information from the former.

// Operation

// ISSUES:
//

type Document = types.Document

// Md5Cache records information about the MD5 cache itself.
type FileSizeCache struct {
	Active                bool             // True if the cache is in use
	Dirty                 bool             // True if the cache has been modified (and should be written out)
	CacheOfPathToFileSize map[string]int64 // A cache of path => file size
}

func main() {

	// TODO vaxhaven_yaml := "bin/vaxhaven.yaml"
	vaxhaven_data := "data/VaxHaven.txt"
	output_file := "vaxhaven.yaml"
	fileSizeCacheFilename := "bin/vaxhaven.filesize.cache"
	fileSizeCacheCreate := true
	verbose := true
	fileSizeCache, err := FileSizeCacheInit(fileSizeCacheFilename, fileSizeCacheCreate)
	if err != nil {
		fmt.Printf("Problem initialising MD5 cache: %+v\n", err)
	}

	documentsMap := ParseNewData(vaxhaven_data, fileSizeCache, verbose)

	// Construct the YAML data and write it out to a file
	data, err := yaml.Marshal(&documentsMap)
	if err != nil {
		log.Fatal(err)
	}

	err = os.WriteFile("bin/"+output_file, data, 0644)
	if err != nil {
		log.Fatal(err)
	}

	// If the File Size cache is active and it has been modified ... write it out
	if fileSizeCache.Active && fileSizeCache.Dirty {
		fmt.Println("Writing File Size cache")
		fileSizeData, err := yaml.Marshal(fileSizeCache.CacheOfPathToFileSize)
		if err != nil {
			log.Fatal("Bad File Size data: ", err)
		}
		err = os.WriteFile(fileSizeCacheFilename, fileSizeData, 0644)
		if err != nil {
			log.Fatal("Failed File Size Cache write: ", err)
		}
	}
}

// Read the bitsavers IndexByDate.txt file and build a set of paths under DEC-related diectories
// that correspond to files with acceptable file types.
// This is so that files that are unlikely to be documents can be filtered out,
// for example file types such as JPG, BIN and so on are not likely to be
// worth recording in a list of documents.

func ParseNewData(filename string, fileSizeCache *FileSizeCache, verbose bool) map[string]Document {

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

	// r := regexp.MustCompile(`(?ms)<tr>(.*?<td>\s*<a\s+href="(.*?)".*?>(.*?)</a>.*?</td>.*?<td>(.*?)</td>.*?<td>(.*?)</td>.*?)</tr>`)
	r_rows := regexp.MustCompile(`(?ms)<tr>(.*?)</tr>`)
	// r_data := regexp.MustCompile(`(?ms)<td>\s*<a\s+href="(.*?)".*?>(.*?)</a>.*?</td>.*?<td>(.*?)</td>.*?<td>(.*?)</td>`)
	r_data := regexp.MustCompile(`(?ms)<td>\s*<a\s+href="(.*?)".*?>(.*?)</a></td>.*?<td>(.*?)</td>.*?<td>(.*?)</td>`)
	r_data_no_date := regexp.MustCompile(`(?ms)<td>\s*<a\s+href="(.*?)".*?>(.*?)</a></td>.*?<td>(.*?)</td>`)
	fmt.Println(r_rows)
	rows := r_rows.FindAllStringSubmatch(string(file), -1)
	// fmt.Println(res)
	fmt.Println("Found ", len(rows), "matches")
	for i, match := range rows {
		//fmt.Println("=============================================================================")
		//fmt.Println("match ", i, " = ", len(match), " => ", match[1])
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
		document := CreateVaxHavenDocument(data[0][1])
		document.PartNum = data[0][2]
		document.Title = data[0][3]
		if len(data[0]) >= 4 {
			document.PubDate = ConvertVaxHavenDate(docDate)
			if document.PubDate == "XXXX" {
				fmt.Printf("Suspicious date [%s] for %s (%s)\n", docDate, document.Title, document.Filepath)
			}
		}
		fileSize, err := CalculatefileSize(document.Filepath, fileSizeCache, verbose)
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

// This function function creates a Document struct with some default values set
func CreateVaxHavenDocument(path string) Document {
	var newDocument Document
	newDocument.Md5 = ""
	newDocument.PubDate = ""
	newDocument.PdfCreator = ""
	newDocument.PdfProducer = ""
	newDocument.PdfVersion = ""
	newDocument.PdfModified = ""
	newDocument.Collection = "vaxhaven"
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

// Prepares the MD% cache for use.
// If no MD5 cache file has been specified, create it if allowed.
// Load YAML data from the cache file (if any).
//
// On exit, if no errors occur, the cache is in a valid state.
func FileSizeCacheInit(fileSizeCacheFilename string, createIfMissing bool) (*FileSizeCache, error) {
	fileSizeCache := new(FileSizeCache)
	fileSizeCache.Active = false
	fileSizeCache.Dirty = false
	fileSizeCache.CacheOfPathToFileSize = make(map[string]int64)
	// If a cache exists, read it; possibly create, it if allowed to do so.
	if fileSizeCacheFilename != "" {
		file, err := os.ReadFile(fileSizeCacheFilename)
		if err != nil {
			if os.IsNotExist(err) {
				if createIfMissing {
					newFile, err := os.Create(fileSizeCacheFilename)
					if err != nil {
						return fileSizeCache, err
					}
					newFile.Close()
					fmt.Printf("Created empty FileSize cache file: %s\n", fileSizeCacheFilename)
					file, err = os.ReadFile(fileSizeCacheFilename)
					if err != nil {
						return fileSizeCache, err
					}
				} else {
					return fileSizeCache, err
				}
			}
		}
		fileSizeCache.Active = true
		// Read the existing cache YAML data into the cache
		err = yaml.Unmarshal(file, fileSizeCache.CacheOfPathToFileSize)
		if err != nil {
			fmt.Println("fileSize cache: failed to unmarshal")
			return fileSizeCache, err
		}
		fmt.Printf("Initial  number of fileSize cache entries: %d\n", len(fileSizeCache.CacheOfPathToFileSize))
	}

	return fileSizeCache, nil
}

// Return the fileSize sum for the specified file.
// Start by looking up the filename (path) in the cache and return a pre-computed fileSize sum if found.
// Otherwise, compute the fileSize sum, add the entry to the cache, mark the cache as dirty and return the computed fileSize sum.
var tempCount int = 0

func CalculatefileSize(filename string, fileSizeCache *FileSizeCache, verbose bool) (int64, error) {

	// Lookup the filename (path) in the cache; if found report that as the fileSize sum
	if fileSize, found := fileSizeCache.CacheOfPathToFileSize[filename]; found {
		if verbose {
			fmt.Printf("fileSize Cache: Found %d for %s\n", fileSize, filename)
		}
		return fileSize, nil
	}
	tempCount += 1
	if tempCount > 10000 {
		fmt.Println("Too many URL lookups")
		return 0, nil
	}

	// The filename (path) is not in the cache.
	// Ask for the remote file size
	url := "http://www.vaxhaven.com" + filename
	resp, err := http.Head(url)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	time.Sleep(2 * time.Second)
	fileSize, _ := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	fileSizeCache.CacheOfPathToFileSize[filename] = fileSize
	fileSizeCache.Dirty = true
	fmt.Printf("fileSize Cache: wrote %d for %s\n", fileSize, filename)
	return fileSize, nil
}
