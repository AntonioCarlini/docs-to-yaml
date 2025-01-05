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
	"reflect"
	"regexp"
	"strings"
	"unicode"

	"gopkg.in/yaml.v2"
)

type Document = document.Document

type PdfMetadata = pdfmetadata.PdfMetadata

// PathAndVolume represents a single local archive.
// PathAndVolume is used when parsing the indirect file.
type PathAndVolume struct {
	Path       string // Path to the root of the local archive
	VolumeName string // Name of the local archive
}

// MissingFile represents the relative path of a missing file.
type MissingFile struct {
	Filepath string
}

// SubstitueFile represents a filename that was incorrectly typed and the file name that should have been typed
type SubstituteFile struct {
	MistypedFilepath string // This is the incorrect filepath (relative to the archive volume root) as entered in an HTML file
	ActualFilepath   string // This is the correct filepath (relative to the archive volume root) that should have been in that HTML file
}

type FileHandlingExceptions struct {
	FileSubstitutes []SubstituteFile
	MissingFiles    []MissingFile
}

type IndirectFileEntry interface{}

type ProgamFlags struct {
	Statistics  bool // display statistics
	Verbose     bool // display extra infomational messages
	GenerateMD5 bool // generate MD5 checksums
	ReadEXIF    bool // Read EXIF data from PDF files
}

// Implement an enum for ArchiveCategory
type ArchiveCategory int

// These are the legal ArchiveCategory enum values
const (
	AC_Undefined ArchiveCategory = iota
	AC_CSV
	AC_Regular
	AC_HTML
	AC_Metadata
	AC_Custom
)

// This turns ArchiveCategory enums into a text string
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
		log.Print("--yaml-output is mandatory - specify an output YAML file")
		fatal_error_seen = true
	}

	if *indirectFile == "" {
		log.Print("--indirect-file is mandatory - specify an input INDIRECT file")
		fatal_error_seen = true
	}

	if fatal_error_seen {
		log.Fatal("Unable to continue because of one or more fatal errors")
	}

	var programFlags ProgamFlags

	programFlags.Statistics = *statistics
	programFlags.Verbose = *verbose
	programFlags.ReadEXIF = *exifRead
	programFlags.GenerateMD5 = *md5Gen

	md5StoreInstantiation := persistentstore.Store[string, string]{}
	md5Store, err := md5StoreInstantiation.Init(*md5CacheFilename, *md5CacheCreate, programFlags.Verbose)
	if err != nil {
		fmt.Printf("Problem initialising MD5 Store: %+v\n", err)
	} else if *verbose {
		fmt.Println("Size of new MD5 store: ", len(md5Store.Data))
	}

	documentsMap := make(map[string]Document)

	indirectFileEntry, err := ParseIndirectFile(*indirectFile)
	if err != nil {
		log.Fatalf("Failed to parse indirect file: %s", err)
	}

	var fileExceptions FileHandlingExceptions

	for _, item := range indirectFileEntry {
		switch t := item.(type) {
		case PathAndVolume:
			extraDocumentsMap := ProcessArchive(item.(PathAndVolume), &fileExceptions, md5Store, programFlags)
			if *verbose {
				for i, doc := range extraDocumentsMap {
					fmt.Println("doc", i, "=>", doc)
				}
				fmt.Println("found ", len(extraDocumentsMap), "new documents")
			}

			for k, v := range extraDocumentsMap {
				key := k
				val, key_exists := documentsMap[k]
				if key_exists {
					if (v.Md5 != "") && (v.Md5 == val.Md5) {
						if *verbose {
							fmt.Printf("WARNING(1a): Document [%s] already exists, identical to original %v (was %v)\n", k, v, val)
						}
					} else {
						fmt.Printf("WARNING(1): Document [%s] in %s already exists (was %s)\n", k, v.Filepath, val.Filepath)
						key = k + "DUPLICATE-of-" + val.Filepath
					}
				}
				documentsMap[key] = v
			}
			if programFlags.Statistics {
				fmt.Printf("Found %4d documents in volume %s\n", len(extraDocumentsMap), item.(PathAndVolume).VolumeName)
			}
		case SubstituteFile:
			fileExceptions.FileSubstitutes = append(fileExceptions.FileSubstitutes, item.(SubstituteFile))
		case MissingFile:
			fileExceptions.MissingFiles = append(fileExceptions.MissingFiles, item.(MissingFile))
		default:
			// Handle unknown types
			fmt.Printf("Unknown type: %v\n", reflect.TypeOf(t))
		}
	}

	if programFlags.Statistics {
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

// ProcessArchive examines a single archive volume, determines the category it belongs to
// and calls the appropriate processing function.
// It returns a map of Document objects that have been found.
func ProcessArchive(archive PathAndVolume, fileExceptions *FileHandlingExceptions, md5Store *persistentstore.Store[string, string], programFlags ProgamFlags) map[string]Document {
	category := DetermineCategory((archive.Path))

	switch category {
	case AC_Undefined:
		fmt.Printf("Cannot process undefined category for %s\n", archive.Path)
	case AC_CSV:
		fmt.Printf("Cannot process CSV category for %s\n", archive.Path)
	case AC_Regular:
		return ParseIndexHtml(archive.Path+"index.htm", archive.VolumeName, archive.Path, fileExceptions, md5Store, programFlags)
	case AC_HTML:
		return ProcessCategoryHTML(archive, fileExceptions, md5Store, programFlags)
	case AC_Metadata:
		return ProcessCategoryMetadata(archive, fileExceptions, md5Store, programFlags)
	case AC_Custom:
		return ProcessCategoryCustom(archive, fileExceptions, md5Store, programFlags)
	}
	return nil
}

func ProcessCategoryHTML(archive PathAndVolume, fileExceptions *FileHandlingExceptions, md5Store *persistentstore.Store[string, string], programFlags ProgamFlags) map[string]Document {
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

	if programFlags.Verbose {
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

	if err != nil {
		fmt.Println("Error walking the path:", err)
		return documentsMap
	}

	// Report whether any directories were found
	if containsDir {
		fmt.Println("HTML/ contains directories.")
	}

	// For each link ... process it
	for _, idx := range links {
		extraDocumentsMap := ParseIndexHtml(archive.Path+idx, archive.VolumeName, archive.Path, fileExceptions, md5Store, programFlags)
		if programFlags.Verbose {
			for i, doc := range extraDocumentsMap {
				fmt.Println("doc", i, "=>", doc)
			}
			fmt.Println("found ", len(extraDocumentsMap), "new documents")
		}
		for k, v := range extraDocumentsMap {
			val, key_exists := documentsMap[k]
			if key_exists {
				if (v.Md5 != "") && (v.Md5 == val.Md5) {
					if programFlags.Verbose {
						fmt.Printf("WARNING(2a): Document [%s] already exists, identical to original %v (was %v)\n", k, v, val)
					}
				} else {
					fmt.Printf("WARNING(2): Document [%s] already exists but being overwritten by %v (was %v)\n", k, v, val)
				}
			}
			documentsMap[k] = v
		}
	}

	return documentsMap
}

func ProcessCategoryMetadata(archive PathAndVolume, fileExceptions *FileHandlingExceptions, md5Store *persistentstore.Store[string, string], programFlags ProgamFlags) map[string]Document {
	// 1. Find all links in index.htm ... each one must point to HTML/XXXX.HTM; build a list of these targets
	// 2. Verify that every file in metadata/ (regardless of filetype) appears in the list of targets
	// process each .HTM file

	// Read index.htm
	indexPath := archive.Path + "index.htm"
	bytes, err := os.ReadFile(indexPath)
	if err != nil {
		log.Fatal(err)
	}

	// Build a list of links found in index.htm
	var links []string
	re := regexp.MustCompile(`(?ms)<TD>\s*<A HREF=\"(.*?)\">\s+(.*?)<\/A>`)
	matches := re.FindAllStringSubmatch(string(bytes), -1)
	if len(matches) == 0 {
		log.Fatalf("No matches found in %s", indexPath)
	} else {
		for _, v := range matches {
			links = append(links, v[1])
		}
	}

	if programFlags.Verbose {
		fmt.Printf("Found %d links in %s\n", len(links), indexPath)
	}

	subdir := archive.Path + "metadata/"

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

	if err != nil {
		fmt.Println("Error walking the path:", err)
		return documentsMap
	}

	// Report whether any directories were found
	if containsDir {
		fmt.Println("metadata/ contains directories.")
	}

	// For each link ... process it
	for _, idx := range links {
		extraDocumentsMap := ParseIndexHtml(archive.Path+idx, archive.VolumeName, archive.Path, fileExceptions, md5Store, programFlags)
		if programFlags.Verbose {
			for i, doc := range extraDocumentsMap {
				fmt.Println("doc", i, "=>", doc)
			}
			fmt.Println("found ", len(extraDocumentsMap), "new documents")
		}
		for k, v := range extraDocumentsMap {
			val, key_exists := documentsMap[k]
			if key_exists {
				var _ = val
				fmt.Printf("WARNING(3): Document [%s] already exists but being overwritten (was %v)\n", k, val)
			}
			documentsMap[k] = v
		}
	}

	return documentsMap
}

// This function processes the one local archive that has an index.htm that both contains links to actual documents but also
// to further .htm files which also contain links to actual documents. Any .htm files in these further .htm files are not
// processed as contains of links but as actual documents.

func ProcessCategoryCustom(archive PathAndVolume, fileExceptions *FileHandlingExceptions, md5Store *persistentstore.Store[string, string], programFlags ProgamFlags) map[string]Document {

	// Read index.htm
	indexPath := archive.Path + "index.htm"
	bytes, err := os.ReadFile(indexPath)
	if err != nil {
		log.Fatal(err)
	}

	documentsMap := make(map[string]Document)

	// Build a list of links found in index.htm
	var links []string
	re := regexp.MustCompile(`(?ms)<TD>\s*<A HREF=\"(.*?)\">\s+(.*?)<\/A>\s*?<TD>\s*(.*?)\s*</TR>`)
	matches := re.FindAllStringSubmatch(string(bytes), -1)
	if len(matches) == 0 {
		log.Fatalf("No matches found in %s", indexPath)
	} else {
		for _, v := range matches {
			target := v[1]
			partNum := v[2]
			title := v[3]
			if strings.HasSuffix(target, ".htm") {
				links = append(links, v[1])
			} else {
				fullFilepath := archive.Path + target
				absoluteFilepath, _ := filepath.Abs(fullFilepath)
				modifiedVolumePath := absoluteFilepath[len(archive.Path):]
				documentPath := "file:///" + "DEC_0040" + "/" + modifiedVolumePath
				// fmt.Println("full=[", fullFilepath, "] abs=[", absoluteFilepath, "] mod=[", modifiedVolumePath, "] a.P=[", archive.Path, "]")
				md5Checksum := ""
				if programFlags.GenerateMD5 {
					md5Checksum, err = CalculateMd5Sum(archive.VolumeName+"//"+modifiedVolumePath, fullFilepath, md5Store, programFlags.Verbose)
					if err != nil {
						log.Fatal(err)
					}
				}
				newDoc := BuildNewLocalDocument(title, partNum, archive.Path+target, documentPath, md5Checksum, programFlags.ReadEXIF)
				key := md5Checksum
				if key == "" {
					key = partNum + "~" + newDoc.Format
					if key == "" {
						key = title + "~" + newDoc.Format
					}
				}
				documentsMap[key] = newDoc
			}
		}
	}

	if programFlags.Verbose {
		fmt.Printf("Found %d links in %s\n", len(links), indexPath)
	}

	if err != nil {
		fmt.Println("Error walking the path:", err)
		return documentsMap
	}

	// Process each .htm link
	for _, idx := range links {
		// Link in index.htm ends in .htm, so process it as a container of links to documents
		extraDocumentsMap := ParseIndexHtml(archive.Path+idx, archive.VolumeName, archive.Path, fileExceptions, md5Store, programFlags)
		if programFlags.Verbose {
			for i, doc := range extraDocumentsMap {
				fmt.Println("doc", i, "=>", doc)
			}
			fmt.Println("found ", len(extraDocumentsMap), "new documents")
		}
		for k, v := range extraDocumentsMap {
			val, key_exists := documentsMap[k]
			if key_exists {
				var _ = val
				fmt.Printf("WARNING(3): Document [%s] already exists but being overwritten (was %v)\n", k, val)
			}
			documentsMap[k] = v
		}
	}

	return documentsMap
}

// Given the path to the root of a document archive, this function works out the
// category that the archive falls into and returns the result.
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
//
//	archive: full-path-to-archive-root archive-name
//
// If full-path-to-HTML-index starts with a double quote, then it ends with one too.
// Note there must be exactly one space between the full-path and the prefix.
func ParseIndirectFile(indirectFile string) ([]IndirectFileEntry, error) {
	var result []IndirectFileEntry

	file, err := os.Open(indirectFile)
	if err != nil {
		return result, err
	}

	defer file.Close()

	regexes := map[*regexp.Regexp]func(string, int) (interface{}, error){
		regexp.MustCompile(`^\s*archive\s*:\s*(.*)$`):            IndirectFileProcessPathAndVolume,
		regexp.MustCompile(`^\s*incorrect-filepath\s*:\s*(.*)$`): IndirectFileProcessSubstituteFilepath,
		regexp.MustCompile(`^\s*truly-missing-file\s*:\s*(.*)$`): IndirectFileProcessMissingFile,
	}

	lineNumber := 0
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		lineNumber += 1

		// Skip empty lines
		if len(line) == 0 {
			continue
		}

		// Skip lines that start with a "#": these are considered to be comments
		if string(line[0]) == "#" {
			continue
		}

		// Iterate over the map of regexes to check if the line matches any known pattern
		foundHandler := false
		for regex, handler := range regexes {
			// If the line matches the regex, call the corresponding handler
			if match := regex.FindStringSubmatch(line); match != nil {
				foundHandler = true

				item, err := handler(match[1], lineNumber)
				if err == nil {
					switch v := item.(type) {
					case PathAndVolume:
						result = append(result, item.(PathAndVolume))
					case SubstituteFile:
						result = append(result, item.(SubstituteFile))
					case MissingFile:
						result = append(result, item.(MissingFile))
					default:
						// Handle unknown types
						fmt.Printf("Unknown type: %v\n", reflect.TypeOf(v))
					}
				}

				break
			}
		}

		if !foundHandler {
			fmt.Printf("Failed to understand line %d [%s] in indirect file %s\n", lineNumber, line, indirectFile)
		}
	}

	return result, nil
}

func IndirectFileProcessPathAndVolume(line string, lineNumber int) (interface{}, error) {
	var result PathAndVolume

	re := regexp.MustCompile(`[^\s"]+|"([^"]*)"`)

	// Break string into sections delimited by white space.
	// However a sequence starting with a double quote will continue until another double quote is seen.
	quotedString := re.FindAllString(line, -1)
	if quotedString == nil {
		return result, fmt.Errorf("indirect file line %d, cannot parse line: [%s])", lineNumber, line)
	} else if len(quotedString) == 1 {
		return result, fmt.Errorf("indirect file line %d, missing volume name (after %s)", lineNumber, quotedString[0])
	}

	q0 := StripOptionalLeadingAndTrailingDoubleQuotes(quotedString[0])
	switch len(quotedString) {
	case 2:
		return PathAndVolume{Path: q0, VolumeName: quotedString[1]}, nil
	case 0:
	case 1:
		return result, fmt.Errorf("indirect file line %d, too few elements: %d", lineNumber, len(quotedString))
	default:
		return result, fmt.Errorf("indirect file line %d, too many elements: %d", lineNumber, len(quotedString))
	}

	return result, fmt.Errorf("indirect file line %d, too many elements: %d", lineNumber, len(quotedString))
}

// This function is called to indicate that a specific filepath refers to a file that is expected not to exist.
// It is only valid for the next volume.
func IndirectFileProcessMissingFile(text string, lineNumber int) (interface{}, error) {
	var result MissingFile
	result.Filepath = text
	return result, nil
}

func IndirectFileProcessSubstituteFilepath(text string, lineNumber int) (interface{}, error) {
	var result SubstituteFile

	re := regexp.MustCompile(`^\s*(.*?)\s+substitute-with\s+(.*)\s*$`)
	match := re.FindStringSubmatch(text)
	if match == nil {
		fmt.Printf("MISMATCH0: IndirectFileProcessSubstituteFilepath(%s, %d)\n", text, lineNumber)
		return result, nil
	} else if len(match) != 3 {
		fmt.Printf("MISMATCH%d: IndirectFileProcessSubstituteFilepath(%s, %d)\n", len(match), text, lineNumber)
		return result, nil
	}
	// Here, exactly the right number of matches
	result.MistypedFilepath = match[1]
	result.ActualFilepath = match[2]

	return result, nil
}

// The index HTML files written to the DVDs are almost all in one of two (similar) formats.
// This function parses any such HTML file to produce a list of files that the index HTML links to
// and the associated part number and title recorded in the index HTML.
// If required then an MD5 checksum is generated and PDF metadata is extracted and recorded.
func ParseIndexHtml(filename string, volume string, root string, fileExceptions *FileHandlingExceptions, md5Store *persistentstore.Store[string, string], programFlags ProgamFlags) map[string]Document {

	if programFlags.Verbose {
		fmt.Println("Processing index for ", filename)
	}
	path := filepath.Dir(filename)
	bytes, err := os.ReadFile(filename)
	if err != nil {
		log.Fatal(err)
	}

	documentsMap := make(map[string]Document)

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
		if programFlags.Verbose {
			fmt.Println("Found", len(title_matches), "documents in HTML")
		}
		for _, match := range title_matches {
			if len(match) != 4 {
				log.Fatal("Bad match")
			} else {
				pathInVolumerelativetoHTML := match[1]
				partNumber := strings.TrimSpace(match[2])
				title := TidyDocumentTitle(match[3])
				fullFilepath := path + "/" + pathInVolumerelativetoHTML
				absoluteFilepath, err := filepath.Abs(fullFilepath)
				modifiedVolumePathInHTML := absoluteFilepath[len(root):]
				if err != nil {
					log.Fatal(err)
				}

				cifp := BuildCaseInsensitivePathGlob(absoluteFilepath)
				candidateFile, err := filepath.Glob(cifp)
				if err != nil {
					log.Fatal(err)
				}
				if len(candidateFile) == 0 {

					// See if the missing file has a substitute filepath, and if so try using that
					fileFound := false
					for idx, v := range fileExceptions.FileSubstitutes {
						if v.MistypedFilepath == modifiedVolumePathInHTML {
							if programFlags.Verbose {
								fmt.Printf("Found in mistyping [%s] in fileExceptions and swapping for %s\n", modifiedVolumePathInHTML, v.ActualFilepath)
							}
							fullFilepath = path + "/" + v.ActualFilepath
							absoluteFilepath, _ = filepath.Abs(fullFilepath)
							cifp := BuildCaseInsensitivePathGlob(absoluteFilepath)
							candidateFile, err = filepath.Glob(cifp)
							if err != nil {
								log.Fatal(err)
							}
							if len(candidateFile) == 0 {
								fmt.Printf("WARNING: Found mistyping [%s] in fileExceptions but swapping for %s (%s), file still not found\n", modifiedVolumePathInHTML, v.ActualFilepath, fullFilepath)
								continue
							} else {
								if programFlags.Verbose {
									fmt.Printf("File found after fixing bad path [%s]  to be %s (%s) in %s\n", modifiedVolumePathInHTML, v.ActualFilepath, fullFilepath, filename)
								}
								fileFound = true
								// Swap the last element into the slot occupied by the now used and to-be-discarded element, then shorten by one
								// This would be quicker (which won't matter in this use case) and is simpler for me to understand 9which does!)
								fileExcLen := len(fileExceptions.FileSubstitutes)
								fileExceptions.FileSubstitutes[idx] = fileExceptions.FileSubstitutes[fileExcLen-1]
								fileExceptions.FileSubstitutes = fileExceptions.FileSubstitutes[:fileExcLen-1]
								break
							}
						}
					}

					// If missing file has not been substituted, see if it is in the set of "missing files"
					fileTrulyMissing := true
					if !fileFound {

						for idx, v := range fileExceptions.MissingFiles {
							if v.Filepath == modifiedVolumePathInHTML {
								fileTrulyMissing = false
								fileExcLen := len(fileExceptions.MissingFiles)
								fileExceptions.MissingFiles[idx] = fileExceptions.MissingFiles[fileExcLen-1]
								fileExceptions.MissingFiles = fileExceptions.MissingFiles[:fileExcLen-1]
							}
						}

						if fileTrulyMissing {
							fmt.Printf("Missing file not mentioned in indirect-file\n")
						}
					}

					// If the missing file is still missing (i.e. not found even if a substitue is available) then skip to avoid generating a document entry
					if !fileFound {
						if fileTrulyMissing {
							log.Printf("MISSING file: %s [%s] linked from %s\n", fullFilepath, modifiedVolumePathInHTML, filename)
						}
						continue
					}

				} else if len(candidateFile) != 1 {
					log.Fatal("Too many files found:", candidateFile)
				}

				// Find the actal pathname withing the volume rather than whatever might have been specified in an HTML file 9which may be the wrong case)
				modifiedVolumePath := candidateFile[0][len(root):]

				// If requested, find the file's MD5 checksum
				md5Checksum := ""
				if programFlags.GenerateMD5 {
					md5Checksum, err = CalculateMd5Sum(volume+"//"+modifiedVolumePath, candidateFile[0], md5Store, programFlags.Verbose)
					if err != nil {
						log.Fatal(err)
					}
				}

				documentRelativePath := "file:///" + volume + "/" + modifiedVolumePath
				newDocument := BuildNewLocalDocument(title, partNumber, candidateFile[0], documentRelativePath, md5Checksum, programFlags.ReadEXIF)

				key := md5Checksum
				if key == "" {
					key = partNumber + "~" + newDocument.Format
					if key == "" {
						key = title + "~" + newDocument.Format
					}
				}

				// If a duplicate is found, keep the previous entry
				if _, ok := documentsMap[key]; ok {
					// If the duplicated entries share the same filepath, then the same file is linked to
					// more than once. This is not a true "conflicting" duplicate, so suppress the report.
					if newDocument.Filepath != documentsMap[key].Filepath {
						previousFilePath := documentsMap[key].Filepath
						// TODO here should warn if warning set and should count duplicates
						// TODO fmt.Println("WARNING(1) Duplicate entry for ", key, " path: ", newDocument.Filepath, " previous: ", previousFilePath)
						newKey := key + "DUPLICATE" + strings.Replace(previousFilePath, "/", "_", 20)
						documentsMap[newKey] = newDocument
					}
				} else {
					documentsMap[key] = newDocument
				}
			}
		}
	}

	if programFlags.Verbose {
		fmt.Printf("Returning %d documents after processing HTML in %s\n", len(documentsMap), filename)
	}

	return documentsMap
}

// This function constructs a Document object with the specified properties.
// Where properties can be derived from a local file, they will be (if permitted).
// MD5 checksum is currently an exception to this and is always supplied.
//
// title:         document title
// partNum:       document part number
// filePath:      path to document
// documentPath:  psudo
// md5Checksum:   MD5 checksum (may be blank)
// readExif:      true if PDF metadata should be extracted, false otherwise
func BuildNewLocalDocument(title string, partNum string, filePath string, documentPath string, md5Checksum string, readExif bool) Document {
	filestats, err := os.Stat(filePath)
	if err != nil {
		log.Fatal(err)
	}

	pdfMetadata := PdfMetadata{}
	if readExif {
		pdfMetadata = pdfmetadata.ExtractPdfMetadata(filePath)
	}

	var newDocument Document
	newDocument.Format = DetermineFileFormat(filePath)
	newDocument.Size = filestats.Size()
	newDocument.Md5 = md5Checksum
	newDocument.Title = strings.TrimSuffix(strings.TrimSpace(title), "\n")
	newDocument.PubDate = "" // Not available anywhere
	newDocument.PartNum = strings.TrimSpace(partNum)
	newDocument.PdfCreator = pdfMetadata.Creator
	newDocument.PdfProducer = pdfMetadata.Producer
	newDocument.PdfVersion = pdfMetadata.Format
	newDocument.PdfModified = pdfMetadata.Modified
	newDocument.Filepath = documentPath
	newDocument.Collection = "local-archive"

	return newDocument
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
	re := regexp.MustCompile(`\s*<BR>(?:\s*<BR>\s*)*\s*`)
	title = re.ReplaceAllString(title, ". ")
	return title
}

// Return the MD5 sum for the specified file.
// Start by looking up the filename (path) in the cache and return a pre-computed MD5 sum if found.
// Otherwise, compute the MD5 sum, add the entry to the cache, mark the cache as dirty and return the computed MD5 sum.
func CalculateMd5Sum(filenameInCache string, fullFilepath string, md5Store *persistentstore.Store[string, string], verbose bool) (string, error) {

	// Lookup the filename (path) in the cache; if found report that as the MD5 sum
	if md5, found := md5Store.Lookup(filenameInCache); found {
		if verbose {
			fmt.Printf("MD5 Store: Found %s for %s\n", md5, filenameInCache)
		}
		return md5, nil
	}

	// The filename (path) is not in the cache.
	// Generate the MD5 sum, add the value to the cache and mark the cache as Dirty
	fileBytes, err := os.ReadFile(fullFilepath)
	if err != nil {
		return "", err
	}
	md5Hash := md5.Sum(fileBytes)
	md5Checksum := hex.EncodeToString(md5Hash[:])
	md5Store.Update(filenameInCache, md5Checksum)
	fmt.Printf("MD5 Store: wrote %s for [%s] (full path %s)\n", md5Checksum, filenameInCache, fullFilepath)
	return md5Checksum, nil
}

// Helper function to remove leading and trailing double quotes, if present.
// Otherwise returns the original string untouched.
func StripOptionalLeadingAndTrailingDoubleQuotes(candidate string) string {
	if len(candidate) == 0 {
		return candidate
	}
	result := candidate
	if (result[0] == '"') && (result[len(result)-1] == '"') {
		result = result[1 : len(result)-1]
		// fmt.Printf("removed quotes from: [%s]\n", candidate)
		// fmt.Printf("result is          :  [%s]\n", result)
	}
	return result
}
