package main

// The intention of this program is to record the information necessary to determine which documents in the local document collection
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
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	types "docs-to-yaml/structs"

	"github.com/barasher/go-exiftool"
	"gopkg.in/yaml.v2"
)

type Document = types.Document

type PathAndVolume struct {
	Path   string
	Volume string
}

// PdfMetdata is used to record a subset of metadata that can be extracted from a PDF file and will be stored in YAML
type PdfMetadata struct {
	Creator  string
	Producer string
	Format   string
	Modified string
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
			p += string(r)
		}
	}
	return p
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

// Determine the file format. This will be TXT, PDF, RNO etc.
// For now, it can just be the filetype, as long as it is one of
// a recognised set. If necessary this could be expanded to use the mimetype
// package.
// Note that "HTM" will be returned as "HTML": both types exist in the collection but it makes no sense to allow both!
var KnownFileTypes = [...]string{"PDF", "TXT", "MEM", "RNO", "PS", "HTM", "HTML", "ZIP", "LN3"}

func DetermineFileFormat(filename string) string {
	filetype := strings.TrimPrefix(strings.ToUpper(filepath.Ext(filename)), ".")
	if filetype == "HTM" {
		filetype = "HTML"
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

// The index HTML files written to the DVDs are almost all in one of two (similar) formats.
// This function parses any such HTML file to produce a list of files that the index HTML links to
// and the associated part number and title recorded in the index HTML.
// If required then an MD5 checksum is generated and PDF metadata is extracted and recorded.
func ParseIndexHtml(filename string, volume string, doMd5 bool, readExif bool) (map[string]Document, map[string]string) {

	fmt.Println("Processing", filename)
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
		fmt.Println("Found", len(title_matches), "documents")
		for _, match := range title_matches {
			if len(match) != 4 {
				log.Fatal("Bad match")
			} else {
				volumePath := match[1]
				partNumber := strings.TrimSpace(match[2])
				title := TidyDocumentTitle(match[3])
				fullFilepath := path + "/" + volumePath
				absolutePath, err := filepath.Abs(fullFilepath)
				if err != nil {
					log.Fatal(err)
				}
				// fmt.Println("ffp=[", fullFilepath, "] abs =[", absolutePath, "]")
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

				md5Checksum := ""
				if doMd5 {
					fileBytes, err := os.ReadFile(candidateFile[0])
					if err != nil {
						log.Fatal(err)
					}
					md5Hash := md5.Sum(fileBytes)
					md5Checksum = hex.EncodeToString(md5Hash[:])
					// fmt.Println("MD5:      ", md5Checksum)
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
					pdfMetadata = ExtractPdfMetadata(candidateFile[0])
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
				newDocument.Filepath = "file:///" + volume + "/" + volumePath
				if _, ok := documentsMap[key]; ok {
					log.Println("Duplicate entry for ", key)
				}
				documentsMap[key] = newDocument
				md5Map[md5Checksum] = candidateFile[0]
			}
		}
	}

	return documentsMap, md5Map
}

// Each line of the indirect file consist of:
// full-path prefix
// If full-path starts with a double quote, then it ends with one too.
// Otherwise there is exactly one space between the full-path and the prefix.
func ParseIndirectFile(indirectFile string) []PathAndVolume {

	file, err := os.Open(indirectFile)
	if err != nil {
		log.Fatalf("failed to open")

	}

	var result []PathAndVolume

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			continue
		}
		if line[0:1] == "\"" {
			re := regexp.MustCompile(`"([^"]+)"\s*(.*)$`)
			quotedString := re.FindStringSubmatch(line)
			fmt.Println("Matched", quotedString)
			//pathPlusVolume := PathAndVolume{ Path: quotedString[1], Volume: quotedString[2] }
			result = append(result, PathAndVolume{Path: quotedString[1], Volume: quotedString[2]})
		} else {
			portions := strings.Split(line, " ")
			// pathPlusVolume := PathAndVolume{Path: portions[0], Volume: portions[1]}
			result = append(result, PathAndVolume{Path: portions[0], Volume: portions[1]})
		}
	}
	return result
}

// Main entry point
func main() {
	verbose := flag.Bool("verbose", false, "Enable verbose reporting")
	yamlOutputFilename := flag.String("yaml", "", "filepath of the output file to hold the generated yaml")
	md5OutputFilename := flag.String("md5", "", "filepath of the output file to hold the generated MD5 => file map")
	exifRead := flag.Bool("exif", false, "Enable EXIF reading")
	indirectFile := flag.String("indirect-file", "", "a file that contains a set of directories to process")

	flag.Parse()

	if *yamlOutputFilename == "" {
		log.Fatal("Please supply a filespec for the output YAML")
	}

	md5Gen := false
	if *md5OutputFilename != "" {
		md5Gen = true
	}

	documentsMap := make(map[string]Document)
	md5Map := make(map[string]string)

	filepathsAndVolumes := ParseIndirectFile(*indirectFile)

	for _, item := range filepathsAndVolumes {
		extraDocumentsMap, extraMd5Map := ParseIndexHtml(item.Path, item.Volume, md5Gen, *exifRead)
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

	data, err := yaml.Marshal(&documentsMap)
	if err != nil {
		log.Fatal("Bad YAML data: ", err)
	}

	err = os.WriteFile(*yamlOutputFilename, data, 0644)
	if err != nil {
		log.Fatal("Failed YAML write: ", err)
	}

	if md5Gen {
		md5Data, err := yaml.Marshal(&md5Map)
		if err != nil {
			log.Fatal("Bad MD5data: ", err)
		}
		err = os.WriteFile(*md5OutputFilename, md5Data, 0644)
		if err != nil {
			log.Fatal("Failed MD5 write: ", err)
		}
	}
}
