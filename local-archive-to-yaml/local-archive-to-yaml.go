package main

// The purpose of this program is to record the information necessary to determine which documents in the locally archived document collection
// are not duplicated in other repositories on the internet.
//
// This program analyses a file tree corresponding to an archived DVD-R (or CD-R) to produce a YAML output that describes
// each of the documents on that archived disc.
//
// The program records these items for every document it finds:
//   o the file path
//   o the document title
//   o the document part number (or sometimes an analogue where no official poart number is evident)
//   o the MD5 checksum
//   o for PDF files, some PDF metadata may be recorded if it can be extracted
//
// The publication date has a field too, but I don't currently have that recorded anywhere and there are a lot of documents!
// Perhaps it may be added as required over time.
//
// The MD5 checksum will help to uniquely identify documents and to spot duplicates. It isn't sufficient however, as, for example,
// documents on bitsavers have changed since I originally downloaded them, even if the changes are only to the metadata. So to
// more reliably spot documents that I have scanned, I record PDF metdata, so that can be used to sort scans.
//
// For background, the local documents were originally archived on DVD-R but now live in various directories on a NAS.
// As there are over 40 locations to scan, this program accepts an "indirect file", which is a list of directories
// to look at (along with a suitable prefix, although that is currently ignored).
//
// OPERATION
//
// All the existing archived optical media has a top-level index.htm that provides a list of documents and their properties
// or links to further HTML files that perform that function. There is some inconsistency in how these index files are laid out
// but that will be resolved over time.
//
// index.txt and index.pdf provide the same information but in a harder to parse form. These files were in fact generated
// from the index.htm and so contain no additional information.
// When present at the top level DEC_NNNN.CRC (where NNNN is the disc number) provides CRC information.
// When present at the top level md5sum provides MD5 information.
//
// The discs were originally produced on a Windows system and as that has a case-insensitive file system it was not noticed
// that the link does not always exactly match the target file in case. So it is necessary to account for this when analysing
// links on an operating system with a case-sensitive file system, such as Linux.
//
// USAGE
//
// Run the program from the repo top level like this:
//
//   go run local-archive-to-yaml/local-archive-to-yaml.go --verbose --md5-cache bin/md5.store  --md5-sum --indirect-file INDIRECT.txt --yaml DOCS.YAML
//
//  --verbose turns on additional messages that may be useful in tracking program operation
//  --md5-sum causes MD5 checksums to be calculated if not already in the store
//  --md5-cache-create allows an MD5 cache to be created if the one specified does not exist
//  --md5-cache indicates where the cache of MD5 data can be found; this will be created if it does not exist and --md5-cache-create is specified and will be updated if --md5-sum is specified
//  --indirect-file indicates the indirect file that specifies which index files to analyse
//  --exif causes PDF metadata to be extracted and stored
//  --yaml-output specifies where the YAML data should be stored
//
// NOTES
//
// To simplify processing, and particularly sanity checking, the following notes split all current acrhived media into a small number of categories.
// As well as simplyfing the indirect file, handling these categories in the code should allow for better sanity checking of the generated data.
//
// All newly archived media (whether on DVD-R or on a NAS etc.) will include a properly formatted index.csv in the root directory and that will provide all the information that this program is trying to create.
// So if index.csv is found, parse it and produce YAML output from that.
//
// If INDEX.HTM is found, all its links will point to .HTM files in HTML/. Each of these .HTM files links to final documents. Any further HTML files found in links are final documents and not further index files.
// As it so happens there is only one archived DVD-R that fits this pattern and it has no further HMTL files anyway.
//
// All other archived media contains an index.htm.
//
// If DEC_0040.CRC exists in the root, then this is a special case.
//
// If the metadata/ subdirectory exists, then all links in index.htm must be to .htm files in metadata/.
// Each of the metadata/????.htm files contains links to documents and never links to further index HTML files.
//
// If there is no metadata/ subdirectory then all links in the root index.htm are to documents that should be indexed.
//

import (
	"bufio"
	"crypto/md5"
	"docs-to-yaml/internal/document"
	"docs-to-yaml/internal/pdfmetadata"
	"docs-to-yaml/internal/persistentstore"
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

type ArchiveCategory int

const (
	AC_Undefined ArchiveCategory = iota
	AC_CSV
	AC_Regular
	AC_HTML
	AC_Metadata
	AC_Custom
)

func (ac ArchiveCategory) String() string {
	return [...]string{"AC_Undefined", "AC_CSV", "AC_Regular", "AC_HTML", "AC_Metadata", "AC_Custom"}[ac]
}

// Main entry point.
// Processes the indirect file.
// For each entry, parses the specified HTML file.
// Finally outputs the cumulative YAML file.
func main() {
	statistics := flag.Bool("statistics", false, "Enable statistics reporting")
	verbose := flag.Bool("verbose", false, "Enable verbose reporting")
	yamlOutputFilename := flag.String("yaml-output", "", "filepath of the output file to hold the generated yaml")
	md5Gen := flag.Bool("md5-sum", false, "Enable generation of MD5 sums")
	exifRead := flag.Bool("exif", false, "Enable EXIF reading")
	indirectFile := flag.String("indirect-file", "", "a file that contains a set of directories to process")
	md5CacheFilename := flag.String("md5-cache", "", "filepath of the file that holds the volume path => MD5sum map")
	md5CacheCreate := flag.Bool("md5-create-cache", false, "allow for the case of a non-existent MD5 cache file")

	flag.Parse()

	fatal_error_seen := false

	if *yamlOutputFilename == "" {
		log.Print("-yaml-output is mandatory - specify an output YAML file")
		fatal_error_seen = true
	}

	if *indirectFile == "" {
		log.Print("--indirect-file is mandatory - specify an input INDIRECT file")
		fatal_error_seen = true
	}

	if fatal_error_seen {
		log.Fatal("Unable to continue becuase of one or more fatal errors")
	}

	md5StoreInstantiation := persistentstore.Store[string, string]{}
	md5Store, err := md5StoreInstantiation.Init(*md5CacheFilename, *md5CacheCreate, *verbose)
	if err != nil {
		fmt.Printf("Problem initialising MD5 Store: %+v\n", err)
	} else if *verbose {
		fmt.Println("Size of new MD5 store: ", len(md5Store.Data))
	}

	documentsMap := make(map[string]Document)
	md5Map := make(map[string]string)

	filepathsAndVolumes, err := ParseIndirectFile(*indirectFile)
	if err != nil {
		log.Fatalf("Failed to parse indirect file: %s", err)
	}

	for _, item := range filepathsAndVolumes {
		extraDocumentsMap, extraMd5Map := ProcessArchive(item, *md5Gen, md5Store, *exifRead, *verbose)
		if *verbose {
			for i, doc := range extraDocumentsMap {
				fmt.Println("doc", i, "=>", doc)
			}
			fmt.Println("found ", len(extraDocumentsMap), "new documents")
		}
		for k, v := range extraDocumentsMap {
			val, key_exists := documentsMap[k]
			if key_exists {
				var _ = val
				fmt.Printf("WARNING: Document [%s] already exists but being overwritten (was %v)\n", k, val)
			}
			documentsMap[k] = v
		}
		for k, v := range extraMd5Map {
			md5Map[k] = v
		}
	}

	if *statistics {
		fmt.Printf("Final tally of %d documents being written to YAML\n", len(documentsMap))
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

	// If the MD5 Store is active and it has been modified ... save it
	md5Store.Save(*md5CacheFilename)
}

func ProcessArchive(archive PathAndVolume, md5Gen bool, md5Store *persistentstore.Store[string, string], exifRead bool, verbose bool) (map[string]Document, map[string]string) {
	category := DetermineCategory((archive.Root))

	switch category {
	case AC_Undefined:
		fmt.Printf("Cannot process undefined category for %s\n", archive.Root)
	case AC_CSV:
		fmt.Printf("Cannot process CSV category for %s\n", archive.Root)
	case AC_Regular:
		return ParseIndexHtml(archive.Path+"index.htm", archive.Volume, archive.Root, md5Gen, md5Store, exifRead, verbose)
	case AC_HTML:
		return ProcessCategoryHTML(archive, md5Gen, md5Store, exifRead, verbose)
	case AC_Metadata:
		fmt.Printf("Cannot process 'metadata' category for %s\n", archive.Root)
	case AC_Custom:
		fmt.Printf("Cannot process 'custom' category for %s\n", archive.Root)
	}
	return nil, nil
}

func ProcessCategoryHTML(archive PathAndVolume, md5Gen bool, md5Store *persistentstore.Store[string, string], exifRead bool, verbose bool) (map[string]Document, map[string]string) {
	// 1. Find all links in INDEX.HTM ... each one must point to HTML/XXXX.HTM; build a list of these targets
	// 2. Verify that every file in HTML/ (regardless of filetype) appears in the list of targets
	// process each .HTM file

	// Read INDEX.HTM
	indexPath := archive.Path + "INDEX.HTM"
	bytes, err := os.ReadFile(indexPath)
	if err != nil {
		log.Fatal(err)
	}

	// Build  alist of links found in INDEX.HTM
	var links []string
	re := regexp.MustCompile(`(?m)<TD>\s*<A HREF=\"(.*?)\">\s+(.*?)<\/A>\s+<\/TD>`)
	matches := re.FindAllStringSubmatch(string(bytes), -1)
	if len(matches) == 0 {
		log.Fatal("No matches found")
	} else {
		for _, v := range matches {
			links = append(links, strings.ToUpper(v[1]))
		}
	}

	if verbose || true /*TOOD*/ {
		fmt.Printf("Found %d links in %s\n", len(links), indexPath)
	}

	subdir := archive.Path + "HTML/"

	var containsDir bool

	// Walk through the directory and its contents
	err = filepath.Walk(subdir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Handle any error that occurs during file walking
			fmt.Println("Error:", err)
			return err
		}
		// Skip the top-level directory itself
		if path == subdir {
			return nil
		}

		// Check if the current path is a directory
		if info.IsDir() {
			// Mark that we have encountered a directory
			containsDir = true
			fmt.Printf("WARNING Found subdirectory %s in %s\n", path, subdir)
			return nil
		}

		// All files in HTML/ should have completely uppercase names
		// if strings.ToUpper(path) != path {
		//	fmt.Printf("WARNING Found not-all-uppercase file %s in %s\n", path, subdir)
		//}

		// TODO
		// All files in HTML/ should appear in links
		// relativePath, err := filepath.Rel(subdir, path)
		//relativePath := path
		//if !links.Contains(relativePath) {
		//	fmt.Printf("WARNING Found not-all-uppercase file %s in %\n", path, subdir)
		//}

		return nil
	})

	documentsMap := make(map[string]Document)
	md5Map := make(map[string]string)

	if err != nil {
		fmt.Println("Error walking the path:", err)
		return documentsMap, md5Map
	}

	// Report whether any directories were found
	if containsDir {
		fmt.Println("HTML/ contains directories.")
	}

	fmt.Printf("Processing category HTML: TBD\n")

	// For each link ... process it
	for _, idx := range links {
		extraDocumentsMap, extraMd5Map := ParseIndexHtml(archive.Path+idx, archive.Volume, archive.Root, md5Gen, md5Store, exifRead, verbose)
		if verbose {
			for i, doc := range extraDocumentsMap {
				fmt.Println("doc", i, "=>", doc)
			}
			fmt.Println("found ", len(extraDocumentsMap), "new documents")
		}
		for k, v := range extraDocumentsMap {
			val, key_exists := documentsMap[k]
			if key_exists {
				var _ = val
				fmt.Printf("WARNING: Document [%s] already exists but being overwritten (was %v)\n", k, val)
			}
			documentsMap[k] = v
		}
		for k, v := range extraMd5Map {
			md5Map[k] = v
		}
	}

	return documentsMap, md5Map
}

// Given the path to the root of a document archive, this function works out the
// category that the rchive falls into and returns the result.
// The category will be used to determine how to process the archive to extract document information.
func DetermineCategory(archiveRoot string) ArchiveCategory {
	// Make sure that archiveRoot has a trailing /
	if archiveRoot[len(archiveRoot)-1:] != "/" {
		archiveRoot += "/"
	}

	found_index_dot_htm := true
	if _, err := os.Stat(archiveRoot + "index.htm"); os.IsNotExist(err) {
		found_index_dot_htm = false
	}

	found_INDEX_dot_HTM := true
	if _, err := os.Stat(archiveRoot + "INDEX.HTM"); os.IsNotExist(err) {
		found_INDEX_dot_HTM = false
	}

	found_custom_indicator := true
	if _, err := os.Stat(archiveRoot + "DEC_0040.CRC"); os.IsNotExist(err) {
		found_custom_indicator = false
	}

	found_dir_HTML := SubdirectoryExists(archiveRoot + "HTML")
	found_dir_metadata := SubdirectoryExists(archiveRoot + "metadata")

	var category ArchiveCategory = AC_Undefined

	valid := true

	if found_INDEX_dot_HTM {
		if !found_dir_HTML {
			fmt.Printf("found INDEX.HTM but no /HTML in %s\n", archiveRoot)
			valid = false
		}
		if found_index_dot_htm || found_dir_metadata || found_custom_indicator {
			fmt.Printf("found INDEX.HTM with one or more of index.htm, metdata/ or DEC_0040.CRC in %s\n", archiveRoot)
			valid = false
		}
		if valid {
			category = AC_HTML
		}
	} else if found_dir_HTML {
		fmt.Printf("found /HTML but no INDEX.HTM in %s\n", archiveRoot)
		valid = false
	}

	if !found_index_dot_htm && category != AC_HTML {
		fmt.Printf("No index.htm found in %s\n", archiveRoot)
		valid = false
	}

	if found_dir_metadata {
		if found_custom_indicator {
			fmt.Printf("Found both metadata/ and DEC_0040.CRC in %s\n", archiveRoot)
			valid = false
		}
		if valid {
			category = AC_Metadata
		}
	}

	if found_custom_indicator {
		if valid {
			category = AC_Custom
		}
	}

	if valid && category == AC_Undefined {
		category = AC_Regular
	}

	// fmt.Printf("index.htm: %-7t  INDEX.HTM: %-7t /HTML: %-7t /metadata: %-7t custom: %-7t cat: %-12s in %s\n", found_index_dot_htm, found_INDEX_dot_HTM, found_dir_HTML, found_dir_metadata, found_custom_indicator, category, archiveRoot)

	return category
}

// Returns true if the specified path is a subdirectory
func SubdirectoryExists(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// Does not exist at all, either as a file or as a directory
		return false
	} else {
		// Check that it is actually a directory
		if fi, err := os.Stat(path); err == nil && fi.IsDir() {
			// Confirmed to be a path
			return true
		} else {
			// Exists but is a file not a path
			return false
		}
	}

}

// Each line of the indirect file consist of:
// full-path-to-archive-root prefix [optional-full-path-to-root]
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
			return result, fmt.Errorf("indirect file line %d, cannot parse line: [%s])", lineNumber, line)
		} else if len(quotedString) == 1 {
			return result, fmt.Errorf("indirect file line %d, missing volume name (after %s)", lineNumber, quotedString[0])
		}

		q0 := StripOptionalLeadingAndTrailingDoubleQuotes(quotedString[0])
		switch len(quotedString) {
		case 2:
			result = append(result, PathAndVolume{Path: q0, Volume: quotedString[1], Root: filepath.Dir(q0)})
		case 3:
			q2 := StripOptionalLeadingAndTrailingDoubleQuotes(quotedString[2])
			result = append(result, PathAndVolume{Path: q0, Volume: quotedString[1], Root: q2})
		default:
			return result, fmt.Errorf("indirect file line %d, too many elements: %d", lineNumber, len(quotedString))
		}
	}
	return result, nil
}

// The index HTML files written to the DVDs are almost all in one of two (similar) formats.
// This function parses any such HTML file to produce a list of files that the index HTML links to
// and the associated part number and title recorded in the index HTML.
// If required then an MD5 checksum is generated and PDF metadata is extracted and recorded.
func ParseIndexHtml(filename string, volume string, root string, doMd5 bool, md5Store *persistentstore.Store[string, string], readExif bool, verbose bool) (map[string]Document, map[string]string) {

	if verbose {
		fmt.Println("Processing index for ", filename)
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
			fmt.Println("Found", len(title_matches), "documents in HTML")
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
					log.Printf("MISSING file: %s linked from %s\n", fullFilepath, filename)
					continue
				} else if len(candidateFile) != 1 {
					log.Fatal("Too many files found:", candidateFile)
				}

				// If requested, find the file's MD5 sum
				md5Checksum := ""
				if doMd5 {
					md5Checksum, err = CalculateMd5Sum(candidateFile[0], md5Store, verbose)
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
				newDocument.Title = strings.TrimSuffix(strings.TrimSpace(title), "\n")
				newDocument.PubDate = "" // Not available anywhere
				newDocument.PartNum = strings.TrimSpace(partNumber)
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

	if verbose {
		fmt.Printf("Returning %d documents after processing HTML in %s\n", len(documentsMap), filename)
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

// Return the MD5 sum for the specified file.
// Start by looking up the filename (path) in the cache and return a pre-computed MD5 sum if found.
// Otherwise, compute the MD5 sum, add the entry to the cache, mark the cache as dirty and return the computed MD5 sum.
func CalculateMd5Sum(filename string, md5Store *persistentstore.Store[string, string], verbose bool) (string, error) {

	// Lookup the filename (path) in the cache; if found report that as the MD5 sum
	if md5, found := md5Store.Lookup(filename); found {
		if verbose {
			fmt.Printf("MD5 Store: Found %s for %s\n", md5, filename)
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
	md5Store.Update(filename, md5Checksum)
	fmt.Printf("MD5 Store: wrote %s for %s\n", md5Checksum, filename)
	return md5Checksum, nil
}

// Helper function to remove leading and trailing double quotes, if present.
// Otherwise returns the original string untouched.
func StripOptionalLeadingAndTrailingDoubleQuotes(candidate string) string {
	result := candidate
	if (result[0] == '"') && (result[len(result)-1] == '"') {
		result = result[1 : len(result)-1]
		// fmt.Printf("removed quotes from: [%s]\n", candidate)
		// fmt.Printf("result is          :  [%s]\n", result)
	}
	return result
}
