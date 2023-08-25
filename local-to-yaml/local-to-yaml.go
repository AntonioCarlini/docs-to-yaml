package main

// The purpose of this program is to record the information necessary to determine which documents in the local document collection
// are not duplicated in other repositories on the internet. A side effect will be to produce an accurate list of local documents
// in a form that makes it easy to perform future analysis.
//
// The intention is to record the file path, the document title, and the part number. The publication date has a field too, but
// I don't currently have that recorded anywhere and there are a lot of documents! Perhaps it may be added as required over time.
//
// The MD5 checksum will help to uniquely identify documents and to spot duplicates. It isn't suffieient however, as, for example,
// documents on bitsavers have changed since I originally downloaded them, even if the changes are only to the metadata. So to
// more reliably spot documents that I have scanned, I record PDF metdata, so that can be used to sort scans.
//
// For background, the local documents were originally archived on DVD-R but now live in various directories on a NAS.
// As there are over 40 locations to scan, this program accepts an "indirect file", which is a list of directories
// to look at (along with a suitable prefix, although that is currently ignored).

import (
	"bufio"
	"crypto/md5"
	"docs-to-yaml/internal/document"
	"docs-to-yaml/internal/pdfmetadata"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"gopkg.in/yaml.v2"
)

type Document = document.Document

type PdfMetadata = pdfmetadata.PdfMetadata

// PathAndVolume is used when parsing the indirect file
type PathAndVolume struct {
	Path   string
	Volume string
	Root   string
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
	yamlOutputFilename := flag.String("yaml", "", "filepath of the output file to hold the generated yaml")
	md5Gen := flag.Bool("md5-sum", false, "Enable generation of MD5 sums")
	exifRead := flag.Bool("exif", false, "Enable EXIF reading")
	indirectFile := flag.String("indirect-file", "", "a file that contains a set of directories to process")
	md5CacheFilename := flag.String("md5-cache", "", "filepath of the file that holds the volume path => MD5sum map")
	md5CacheCreate := flag.Bool("md5-create-cache", false, "allow for the case of a non-existent MD5 cache file")

	flag.Parse()

	if *yamlOutputFilename == "" {
		log.Fatal("Please supply a filespec for the output YAML")
	}

	md5Cache, err := Md5CacheInit(*md5CacheFilename, *md5CacheCreate)
	if err != nil {
		fmt.Printf("Problem initialising MD5 cache: %+v\n", err)
	}

	documentsMap := make(map[string]Document)
	md5Map := make(map[string]string)

	filepathsAndVolumes, err := ParseIndirectFile(*indirectFile)
	if err != nil {
		log.Fatalf("Failed to parse indirect file: %s", err)
	}

	for _, item := range filepathsAndVolumes {
		extraDocumentsMap, extraMd5Map := ParseIndexHtml(item.Path, item.Volume, item.Root, *md5Gen, md5Cache, *exifRead, *verbose)
		if *verbose {
			for i, doc := range documentsMap {
				fmt.Println("doc", i, "=>", doc)
			}
			fmt.Println("found ", len(documentsMap), "documents")
		}
		for k, v := range extraDocumentsMap {
			documentsMap[k] = v
		}
		for k, v := range extraMd5Map {
			md5Map[k] = v
		}
	}

	// Write the output YAML file
	data, err := yaml.Marshal(&documentsMap)
	if err != nil {
		log.Fatal("Bad YAML data: ", err)
	}

	err = os.WriteFile(*yamlOutputFilename, data, 0644)
	if err != nil {
		log.Fatal("Failed YAML write: ", err)
	}

	// If the MD5 cache is active and it has been modified ... write it out
	if md5Cache.Active && md5Cache.Dirty {
		fmt.Println("Writing MD5 cache")
		md5Data, err := yaml.Marshal(md5Cache.CacheOfPathToMd5)
		if err != nil {
			log.Fatal("Bad MD5data: ", err)
		}
		err = os.WriteFile(*md5CacheFilename, md5Data, 0644)
		if err != nil {
			log.Fatal("Failed MD5 write: ", err)
		}
	}
}

// Each line of the indirect file consist of:
// full-path-to-HTML-index prefix [optional-full-path-to-root]
//
// If full-path-to-HTML-index starts with a double quote, then it ends with one too.
// The same would be true of optional-full-path-to-root, but that has not been implemented.
// Otherwise there is exactly one space between the full-path and the prefix.
func ParseIndirectFile(indirectFile string) ([]PathAndVolume, error) {
	var result []PathAndVolume

	file, err := os.Open(indirectFile)
	if err != nil {
		return result, err
	}

	lineNumber := 0
	scanner := bufio.NewScanner(file)
	re := regexp.MustCompile(`[^\s"]+|"([^"]*)"`)
	for scanner.Scan() {
		line := scanner.Text()
		lineNumber += 1
		if len(line) == 0 {
			continue
		}
		quotedString := re.FindAllString(line, -1)
		if quotedString == nil {
			return result, fmt.Errorf("indirect file line %d, cannot parse line: [%s])\n", lineNumber, line)
		} else if len(quotedString) == 1 {
			return result, fmt.Errorf("indirect file line %d, missing volume name (after %s)\n", lineNumber, quotedString[0])
		}

		q0 := StripOptionalLeadingAndTrailingDoubleQuotes(quotedString[0])
		switch len(quotedString) {
		case 2:
			result = append(result, PathAndVolume{Path: q0, Volume: quotedString[1], Root: filepath.Dir(q0)})
		case 3:
			q2 := StripOptionalLeadingAndTrailingDoubleQuotes(quotedString[2])
			result = append(result, PathAndVolume{Path: q0, Volume: quotedString[1], Root: q2})
		default:
			return result, fmt.Errorf("indirect file line %d, too many elements: %d\n", lineNumber, len(quotedString))
		}
	}
	return result, nil
}

// The index HTML files written to the DVDs are almost all in one of two (similar) formats.
// This function parses any such HTML file to produce a list of files that the index HTML links to
// and the associated part number and title recorded in the index HTML.
// If required then an MD5 checksum is generated and PDF metadata is extracted and recorded.
func ParseIndexHtml(filename string, volume string, root string, doMd5 bool, md5Cache *Md5Cache, readExif bool, verbose bool) (map[string]Document, map[string]string) {

	if verbose {
		fmt.Println("Processing", filename)
	}
	path := filepath.Dir(filename)
	bytes, err := os.ReadFile(filename)
	if err != nil {
		log.Fatal(err)
	}

	documentsMap := make(map[string]Document)
	md5Map := make(map[string]string)

	// Each entry we care about looks like this:
	//	<TR VALIGN=TOP>
	//	<TD> <A HREF="decmate/ssm.txt"> DEC-S8-OSSMB-A-D
	//	<TD> OS/8 SOFTWARE SUPPORT MANUAL
	//	</TR>
	//
	// Exceptionally, an entry in DEC_0002 looks like this:
	// <TR> <TD VALIGN=TOP>
	// <A HREF="../manuals/internal/pvaxfw.pdf"> PVAX FW </A>
	// <TD> Functional Specification for PVAX0 System Firmware Rev 0.3</TR>

	re := regexp.MustCompile(`(?ms)<TR(?:>\s*<TD)?\s+VALIGN=TOP>.*?(?:<TD>)?\s*<A HREF=\"(.*?)\">\s+(.*?)(?:</A>)?\s+<TD>\s+(.*?)</TR>`)
	title_matches := re.FindAllStringSubmatch(string(bytes), -1)
	if len(title_matches) == 0 {
		log.Fatal("No matches found")
	} else {
		if verbose {
			fmt.Println("Found", len(title_matches), "documents")
		}
		for _, match := range title_matches {
			if len(match) != 4 {
				log.Fatal("Bad match")
			} else {
				volumePath := match[1]
				partNumber := strings.TrimSpace(match[2])
				title := TidyDocumentTitle(match[3])
				fullFilepath := path + "/" + volumePath
				absolutePath, err := filepath.Abs(fullFilepath)
				// fmt.Println("abs=", absolutePath, "root=", root)
				modifiedVolumePath := absolutePath[len(root):]
				if err != nil {
					log.Fatal(err)
				}
				// fmt.Println("path=", path, "ffp=[", fullFilepath, "] abs =[", absolutePath, "]")
				cifp := BuildCaseInsensitivePathGlob(absolutePath)
				candidateFile, err := filepath.Glob(cifp)
				if err != nil {
					log.Fatal(err)
				}
				if len(candidateFile) == 0 {
					log.Println("No file found:", fullFilepath)
					continue
				} else if len(candidateFile) != 1 {
					log.Fatal("Too many files found:", candidateFile)
				}

				// If requested, find the file's MD5 sum
				md5Checksum := ""
				if doMd5 {
					md5Checksum, err = CalculateMd5Sum(candidateFile[0], md5Cache, verbose)
					if err != nil {
						log.Fatal(err)
					}
				}
				key := md5Checksum
				if key == "" {
					key = partNumber
					if key == "" {
						key = title
					}
				}

				filestats, err := os.Stat(candidateFile[0])
				if err != nil {
					log.Fatal(err)
				}

				pdfMetadata := PdfMetadata{}
				if readExif {
					pdfMetadata = pdfmetadata.ExtractPdfMetadata(candidateFile[0])
				}

				var newDocument Document
				newDocument.Format = DetermineFileFormat(volumePath)
				newDocument.Size = filestats.Size()
				newDocument.Md5 = md5Checksum
				newDocument.Title = title
				newDocument.PubDate = "" // Not available anywhere
				newDocument.PartNum = partNumber
				newDocument.PdfCreator = pdfMetadata.Creator
				newDocument.PdfProducer = pdfMetadata.Producer
				newDocument.PdfVersion = pdfMetadata.Format
				newDocument.PdfModified = pdfMetadata.Modified
				newDocument.Filepath = "file:///" + volume + "/" + modifiedVolumePath
				// If a duplicate is found, keep the previous entry
				if _, ok := documentsMap[key]; ok {
					// If the duplicated entries share the same filepath, then the same file is linked to
					// more than once. This is not a true "conflicting" duplicate, so suppress the report.
					if newDocument.Filepath != documentsMap[key].Filepath {
						log.Println("Duplicate entry for ", key, " path: ", newDocument.Filepath, " previous: ", documentsMap[key].Filepath)
					}
				} else {
					documentsMap[key] = newDocument
				}
			}
		}
	}

	return documentsMap, md5Map
}

// The index HTML files written to the various DVDs were tested on a Windows system, which performs case-insensitive
// filename matching. Linux has no way to perform case-insensitive matching. So this funcion turns each letter in the
// putative filepath into a regexp expression that matches either the uppercase of the lowercase version of that
// letter.
func BuildCaseInsensitivePathGlob(path string) string {
	p := ""
	for _, r := range path {
		if unicode.IsLetter(r) {
			p += fmt.Sprintf("[%c%c]", unicode.ToLower(r), unicode.ToUpper(r))
		} else {
			if (r == '[') || (r == ']') {
				p += "\\" + string(r)
			} else {
				p += string(r)
			}
		}
	}
	return p
}

// Determine the file format. This will be TXT, PDF, RNO etc.
// For now, it can just be the filetype, as long as it is one of
// a recognised set. If necessary this could be expanded to use the mimetype
// package.
// Note that "HTM" will be returned as "HTML": both types exist in the collection but it makes no sense to allow both!
// Similarly "JPG" will be returned as "JPEG".
var KnownFileTypes = [...]string{"PDF", "TXT", "MEM", "RNO", "PS", "HTM", "HTML", "ZIP", "LN3", "TIF", "JPG", "JPEG"}

func DetermineFileFormat(filename string) string {
	filetype := strings.TrimPrefix(strings.ToUpper(filepath.Ext(filename)), ".")
	if filetype == "HTM" {
		filetype = "HTML"
	}
	if filetype == "JPE" {
		filetype = "JPEG"
	}

	for _, entry := range KnownFileTypes {
		if entry == filetype {
			return filetype
		}
	}
	log.Fatal("Unknown filetype: ", filetype)
	return "???"
}

// Clean up a document title that has been read from HTML.
//
//	o remove leading/trailing whitespace
//	o remove CRLF
//	o collapse duplicate whitespace
//	o replace "<BR><BR>", " <BR>" and "<BR>" with something sensible
func TidyDocumentTitle(untidyTitle string) string {
	title := strings.TrimSpace(untidyTitle)
	title = strings.Replace(title, "\r\n", "", -1)
	title = strings.Join(strings.Fields(title), " ") // Collapse duplicate whitespace
	re := regexp.MustCompile(`\s*<BR>(?:<BR>)*`)
	title = re.ReplaceAllString(title, ". ")
	return title
}

// Prepares the MD% cache for use.
// If no MD5 cache file has been specified, create it if allowed.
// Load YAML data from the cache file (if any).
//
// On exit, if no errors occur, the cache is in a valid state.
func Md5CacheInit(md5CacheFilename string, createIfMissing bool) (*Md5Cache, error) {
	md5Cache := new(Md5Cache)
	md5Cache.Active = false
	md5Cache.Dirty = false
	md5Cache.CacheOfPathToMd5 = make(map[string]string)
	// If a cache exists, read it; possibly create, it if allowed to do so.
	if md5CacheFilename != "" {
		file, err := os.ReadFile(md5CacheFilename)
		if err != nil {
			if os.IsNotExist(err) {
				if createIfMissing {
					newFile, err := os.Create(md5CacheFilename)
					if err != nil {
						return md5Cache, err
					}
					newFile.Close()
					fmt.Printf("Created empty cache file: %s\n", md5CacheFilename)
					file, err = os.ReadFile(md5CacheFilename)
					if err != nil {
						return md5Cache, err
					}
				} else {
					return md5Cache, err
				}
			}
		}
		md5Cache.Active = true
		// Read the existing cache YAML data into the cache
		err = yaml.Unmarshal(file, md5Cache.CacheOfPathToMd5)
		if err != nil {
			fmt.Println("MD5 cache: failed to unmarshal")
			return md5Cache, err
		}
		fmt.Printf("Initial  number of MD5 cache entries: %d\n", len(md5Cache.CacheOfPathToMd5))
	}

	return md5Cache, nil
}

// Return the MD5 sum for the specified file.
// Start by looking up the filename (path) in the cache and return a pre-computed MD5 sum if found.
// Otherwise, compute the MD5 sum, add the entry to the cache, mark the cache as dirty and return the computed MD5 sum.
func CalculateMd5Sum(filename string, md5Cache *Md5Cache, verbose bool) (string, error) {

	// Lookup the filename (path) in the cache; if found report that as the MD5 sum
	if md5, found := md5Cache.CacheOfPathToMd5[filename]; found {
		if verbose {
			fmt.Printf("MD5 Cache: Found %s for %s\n", md5, filename)
		}
		return md5, nil
	}

	// The filename (path) is not in the cache.
	// Generate the MD5 sum, add the value to the cache and mark the cache as Dirty
	fileBytes, err := os.ReadFile(filename)
	if err != nil {
		return "", err
	}
	md5Hash := md5.Sum(fileBytes)
	md5Checksum := hex.EncodeToString(md5Hash[:])
	md5Cache.CacheOfPathToMd5[filename] = md5Checksum
	md5Cache.Dirty = true
	fmt.Printf("MD5 Cache: wrote %s for %s\n", md5Checksum, filename)
	return md5Checksum, nil
}

// helper function to remove leading and trailing double quotes, if present.
// Otherwise returns the original string untouched.
func StripOptionalLeadingAndTrailingDoubleQuotes(candidate string) string {
	result := candidate
	if (result[0] == '"') && (result[len(result)-1] == '"') {
		result = result[1 : len(result)-1]
		fmt.Printf("removed quotes from: [%s]\n", candidate)
		fmt.Printf("result is          :  [%s]\n", result)
	}
	return result
}
