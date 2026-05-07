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
//   go run local-archive-to-yaml/local-archive-to-yaml.go --verbose --md5-cache cache/md5.store  --md5-sum --indirect-file INDIRECT.txt --yaml DOCS.YAML
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
	"bytes"
	"crypto/md5"
	"docs-to-yaml/internal/document"
	"docs-to-yaml/internal/pdfmetadata"
	"docs-to-yaml/internal/persistentstore"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path" // Used for internal FS paths
	"reflect"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/filesystem/iso9660"
)

type Document = document.Document

type PdfMetadata = pdfmetadata.PdfMetadata

// PathAndVolume represents a single local archive.
// PathAndVolume is used when parsing the indirect file.
type PathAndVolume struct {
	Path       string // Path to the root of the local archive
	VolumeName string // Name of the local archive
}

type NrgPathAndVolume struct {
	NrgPath    string // Path to the .nrg file itself
	VolumeName string // The name used for YAML collection labeling
}

// MissingFile represents the relative path of a missing file.
type MissingFile struct {
	Filepath string
}

// SubstituteFile represents a filename that was incorrectly typed and the file name that should have been typed
type SubstituteFile struct {
	MistypedFilepath string // This is the incorrect filepath (relative to the archive volume root) as entered in an HTML file
	ActualFilepath   string // This is the correct filepath (relative to the archive volume root) that should have been in that HTML file
}

type FileHandlingExceptions struct {
	FileSubstitutes []SubstituteFile
	MissingFiles    []MissingFile
	ResolvedPathMap map[string]string // key: resolved path (volume-scoped), value: original HTML link
}

type IndirectFileEntry interface{}

type ProgamFlags struct {
	Statistics  bool // display statistics
	Verbose     bool // display extra infomational messages
	GenerateMD5 bool // generate MD5 checksums
	ReadEXIF    bool // Read EXIF data from PDF files
}

type IsoPathAndVolumeWithGoDiskFS struct {
	IsoPath    string // Path to the .iso file itself
	MountPoint string // Where you want it mounted
	VolumeName string // The name used for YAML collection labeling
}

type IsoPathAndVolumeBsdTar struct {
	IsoPath    string // Path to the .iso file itself
	VolumeName string // The name used for YAML collection labeling
}

//type BsdTarFS struct {
//	isoPath string
//	files   map[string]bool     // path → isDir
//	tree    map[string][]string // dir → children
//}

type bsdFileInfo struct {
	name    string
	size    int64
	isDir   bool
	mode    fs.FileMode
	modTime time.Time
}
type BsdTarFS struct {
	isoPath string
	files   map[string]*bsdFileInfo // path → file info
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
			// Create an fs.FS from a local directory
			fsys := os.DirFS(t.Path)

			// Call the same ProcessArchive function
			extraDocumentsMap := ProcessArchive(fsys, t, &fileExceptions, md5Store, programFlags)
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
				fmt.Printf("Found %4d documents in volume %s\n", len(extraDocumentsMap), t.VolumeName)
			}
		case IsoPathAndVolumeWithGoDiskFS:
			// Open the ISO using go-diskfs
			disk, err := diskfs.Open(t.IsoPath, diskfs.WithOpenMode(diskfs.ReadOnly))
			if err != nil {
				log.Fatalf("failed to open ISO: %s", err)
			}

			// Get the ISO9660 filesystem
			// Attempt to get the filesystem.
			// -1 tells go-diskfs to look at the whole disk if no partition table exists.
			fsys, err := disk.GetFilesystem(-1)
			if err != nil {
				// Manual fallback for bare ISOs
				fsys = &iso9660.FileSystem{} // This requires more complex initialization
				// Stick to the -1 vs 0 logic first as it is the standard wrapper
			}

			// Map the virtual "/" of the ISO as the path
			extraDocumentsMap := ProcessArchive(fsys, PathAndVolume{Path: ".", VolumeName: t.VolumeName}, &fileExceptions, md5Store, programFlags)

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
				fmt.Printf("Found %4d documents in volume %s\n", len(extraDocumentsMap), t.VolumeName)
			}
		case IsoPathAndVolumeBsdTar:
			// NEW: bsdtar-based handling
			fsys, err := NewBsdTarFS(t.IsoPath) // from the corrected BsdTarFS implementation
			if err != nil {
				log.Fatalf("Failed to create BsdTarFS for %s: %v", t.IsoPath, err)
			}
			// Use a dummy mount point (ignored)
			archive := PathAndVolume{Path: ".", VolumeName: t.VolumeName}
			extraDocumentsMap := ProcessArchive(fsys, archive, &fileExceptions, md5Store, programFlags)
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
				fmt.Printf("Found %4d documents in volume %s\n", len(extraDocumentsMap), t.VolumeName)
			}
		case NrgPathAndVolume:
			// Create a temporary mount point
			mountPoint, err := os.MkdirTemp("", "nrg_mount_*")
			if err != nil {
				log.Fatalf("Failed to create temp dir for NRG mount: %v", err)
			}
			// Mount using fuseiso
			cmd := exec.Command("fuseiso", "-p", t.NrgPath, mountPoint)
			if err := cmd.Run(); err != nil {
				os.RemoveAll(mountPoint)
				log.Fatalf("Failed to mount NRG %s: %v", t.NrgPath, err)
			}
			// Ensure unmount and cleanup
			defer func() {
				// Unmount with fusermount
				umountCmd := exec.Command("fusermount", "-u", mountPoint)
				if err := umountCmd.Run(); err != nil {
					log.Printf("Warning: failed to unmount %s: %v", mountPoint, err)
				}
				os.RemoveAll(mountPoint)
			}()
			// Process as normal directory
			fsys := os.DirFS(mountPoint)
			archive := PathAndVolume{Path: mountPoint, VolumeName: t.VolumeName}
			extraDocumentsMap := ProcessArchive(fsys, archive, &fileExceptions, md5Store, programFlags)
			// Merge results (same as PathAndVolume case)
			if programFlags.Verbose {
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
						if programFlags.Verbose {
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
				fmt.Printf("Found %4d documents in volume %s\n", len(extraDocumentsMap), t.VolumeName)
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

	// If the MD5 Store is active and it has been modified ... save it
	md5Store.Save(*md5CacheFilename)

	// Write the output YAML file
	err = document.WriteDocumentsMapToOrderedYaml(documentsMap, *yamlOutputFilename)
	if err != nil {
		log.Fatal("Failed YAML write: ", err)
	}

}

// ProcessArchive examines a single archive volume, determines the category it belongs to
// and calls the appropriate processing function.
// It returns a map of Document objects that have been found.
func ProcessArchive(fsys fs.FS, archive PathAndVolume, fileExceptions *FileHandlingExceptions, md5Store *persistentstore.Store[string, string], programFlags ProgamFlags) map[string]Document {
	category := DetermineCategory(fsys, archive.VolumeName)

	switch category {
	case AC_Undefined:
		fmt.Printf("Cannot process undefined category for volume %s\n", archive.VolumeName)
	case AC_CSV:
		fmt.Printf("Cannot process CSV category for volume %s\n", archive.VolumeName)
	case AC_Regular:
		return ParseIndexHtml(fsys, "index.htm", archive.VolumeName, archive.Path, fileExceptions, md5Store, programFlags)
	case AC_HTML:
		return ProcessCategoryHTML(fsys, archive, fileExceptions, md5Store, programFlags)
	case AC_Metadata:
		return ProcessCategoryMetadata(fsys, archive, fileExceptions, md5Store, programFlags)
	case AC_Custom:
		return ProcessCategoryCustom(fsys, archive, fileExceptions, md5Store, programFlags)
	}
	return nil
}

func ProcessCategoryHTML(fsys fs.FS, archive PathAndVolume, fileExceptions *FileHandlingExceptions, md5Store *persistentstore.Store[string, string], programFlags ProgamFlags) map[string]Document {
	// 1. Find all links in INDEX.HTM ... each one must point to HTML/XXXX.HTM; build a list of these targets
	// 2. Verify that every file in HTML/ (regardless of filetype) appears in the list of targets
	// process each .HTM file

	// Read INDEX.HTM from virtual FS
	bytes, err := fs.ReadFile(fsys, "INDEX.HTM")
	if err != nil {
		log.Fatal(err)
	}

	// Build a list of links found in INDEX.HTM
	var links []string
	re := regexp.MustCompile(`(?m)<TD>\s*<A HREF=\"(.*?)\">\s+(.*?)<\/A>\s+<\/TD>`)
	matches := re.FindAllStringSubmatch(string(bytes), -1)
	if len(matches) == 0 {
		log.Fatal("No matches found")
	} else {
		for _, v := range matches {
			links = append(links, v[1])
		}
	}

	if programFlags.Verbose {
		fmt.Printf("Found %d links in INDEX.HTM (Volume %s)\n", len(links), archive.VolumeName)
	}

	// Walk through the directory using virtual FS
	err = fs.WalkDir(fsys, "HTML", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			fmt.Println("Error:", err)
			return err
		}
		if d.IsDir() && path != "HTML" {
			fmt.Printf("WARNING Found subdirectory %s in HTML/\n", path)
		}
		return nil
	})

	documentsMap := make(map[string]Document)

	if err != nil {
		fmt.Println("Error walking the path:", err)
		return documentsMap
	}

	// For each link ... process it
	for _, link := range links {
		// Resolve the link case-insensitively (e.g., html/alpha.htm -> HTML/ALPHA.HTM)
		resolvedLink, err := resolvePathCaseInsensitive(fsys, link)
		if err != nil {
			// If we can't find the sub-index file, log it and move to the next one
			log.Printf("Warning: Sub-index link %s not found in Volume %s", link, archive.VolumeName)
			continue
		}

		// Now ParseIndexHtml will receive a filename that fs.ReadFile actually understands
		extraDocumentsMap := ParseIndexHtml(fsys, resolvedLink, archive.VolumeName, archive.Path, fileExceptions, md5Store, programFlags)
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

func ProcessCategoryMetadata(fsys fs.FS, archive PathAndVolume, fileExceptions *FileHandlingExceptions, md5Store *persistentstore.Store[string, string], programFlags ProgamFlags) map[string]Document {
	// 1. Find all links in index.htm ... each one must point to HTML/XXXX.HTM; build a list of these targets
	// 2. Verify that every file in metadata/ (regardless of filetype) appears in the list of targets
	// process each .HTM file

	// Read index.htm from virtual FS
	bytes, err := fs.ReadFile(fsys, "index.htm")
	if err != nil {
		log.Fatal(err)
	}

	// Build a list of links found in index.htm
	var links []string
	re := regexp.MustCompile(`(?ms)<TD>\s*<A HREF=\"(.*?)\">\s+(.*?)<\/A>`)
	matches := re.FindAllStringSubmatch(string(bytes), -1)
	if len(matches) == 0 {
		log.Fatalf("No matches found in index.htm for Volume %s", archive.VolumeName)
	} else {
		for _, v := range matches {
			links = append(links, v[1])
		}
	}

	if programFlags.Verbose {
		fmt.Printf("Found %d links in index.htm (Volume %s)\n", len(links), archive.VolumeName)
	}

	// Walk through metadata using virtual FS
	err = fs.WalkDir(fsys, "metadata", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			fmt.Println("Error:", err)
			return err
		}
		if d.IsDir() && path != "metadata" {
			fmt.Printf("WARNING Found subdirectory %s in metadata/\n", path)
		}
		return nil
	})

	documentsMap := make(map[string]Document)

	if err != nil {
		fmt.Println("Error walking the path:", err)
		return documentsMap
	}

	// For each link ... process it
	for _, idx := range links {
		extraDocumentsMap := ParseIndexHtml(fsys, idx, archive.VolumeName, archive.Path, fileExceptions, md5Store, programFlags)
		for k, v := range extraDocumentsMap {
			val, key_exists := documentsMap[k]
			if key_exists {
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
func ProcessCategoryCustom(fsys fs.FS, archive PathAndVolume, fileExceptions *FileHandlingExceptions, md5Store *persistentstore.Store[string, string], programFlags ProgamFlags) map[string]Document {
	// Read index.htm from virtual FS
	bytes, err := fs.ReadFile(fsys, "index.htm")
	if err != nil {
		log.Fatal(err)
	}

	documentsMap := make(map[string]Document)

	// Build a list of links found in index.htm
	var links []string
	re := regexp.MustCompile(`(?ms)<TD>\s*<A HREF=\"(.*?)\">\s+(.*?)<\/A>\s*?<TD>\s*(.*?)\s*</TR>`)
	matches := re.FindAllStringSubmatch(string(bytes), -1)
	if len(matches) == 0 {
		log.Fatalf("No matches found in custom index.htm for Volume %s", archive.VolumeName)
	} else {
		for _, v := range matches {
			target := v[1]
			partNum := v[2]
			title := v[3]
			if strings.HasSuffix(target, ".htm") {
				links = append(links, target)
			} else {
				md5Checksum := ""
				if programFlags.GenerateMD5 {
					md5Checksum, err = CalculateMd5Sum(fsys, archive.VolumeName+"//"+target, target, md5Store, programFlags.Verbose)
					if err != nil {
						log.Fatal(err)
					}
				}
				documentPath := "file:///" + archive.VolumeName + "/" + target
				newDoc := BuildNewLocalDocument(fsys, title, partNum, target, documentPath, md5Checksum, programFlags.ReadEXIF)
				newDoc.Collection = "local:" + archive.VolumeName
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

	for _, idx := range links {
		extraDocumentsMap := ParseIndexHtml(fsys, idx, archive.VolumeName, archive.Path, fileExceptions, md5Store, programFlags)
		for k, v := range extraDocumentsMap {
			if _, exists := documentsMap[k]; exists {
				fmt.Printf("WARNING(3): Document [%s] already exists but being overwritten\n", k)
			}
			documentsMap[k] = v
		}
	}

	return documentsMap
}

// Given the path to the root of a document archive, this function works out the
// category that the archive falls into and returns the result.
// The category will be used to determine how to process the archive to extract document information.
func DetermineCategory(fsys fs.FS, volumeName string) ArchiveCategory {
	// Use fs.Stat for virtual filesystem compatibility
	found_index_dot_htm := true
	if _, err := fs.Stat(fsys, "index.htm"); err != nil {
		found_index_dot_htm = false
	}

	found_INDEX_dot_HTM := true
	if _, err := fs.Stat(fsys, "INDEX.HTM"); err != nil {
		found_INDEX_dot_HTM = false
	}

	found_custom_indicator := true
	if _, err := fs.Stat(fsys, "DEC_0040.CRC"); err != nil {
		found_custom_indicator = false
	}

	found_dir_HTML := false
	if fi, err := fs.Stat(fsys, "HTML"); err == nil && fi.IsDir() {
		found_dir_HTML = true
	}

	found_dir_metadata := false
	if fi, err := fs.Stat(fsys, "metadata"); err == nil && fi.IsDir() {
		found_dir_metadata = true
	}

	var category ArchiveCategory = AC_Undefined
	valid := true

	if found_INDEX_dot_HTM {
		if !found_dir_HTML {
			fmt.Printf("found INDEX.HTM but no /HTML in Volume %s\n", volumeName)
			valid = false
		}
		if found_index_dot_htm || found_dir_metadata || found_custom_indicator {
			fmt.Printf("found INDEX.HTM with conflicting files in Volume %s\n", volumeName)
			valid = false
		}
		if valid {
			category = AC_HTML
		}
	} else if found_dir_HTML {
		fmt.Printf("found /HTML but no INDEX.HTM in Volume %s\n", volumeName)
		valid = false
	}

	if !found_index_dot_htm && category != AC_HTML {
		fmt.Printf("No index.htm found in Volume %s\n", volumeName)
		valid = false
	}

	if found_dir_metadata {
		if found_custom_indicator {
			fmt.Printf("Found both metadata/ and DEC_0040.CRC in Volume %s\n", volumeName)
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

	return category
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
		regexp.MustCompile(`^\s*iso-godiskfs\s*:\s*(.*)$`):       IndirectFileProcessIsoPathGoDiskFS,
		regexp.MustCompile(`^\s*iso-bsdtar\s*:\s*(.*)$`):         IndirectFileProcessIsoPathBsdTar,
		regexp.MustCompile(`^\s*iso-nrg\s*:\s*(.*)$`):            IndirectFileProcessIsoPathNrg,
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
					case IsoPathAndVolumeWithGoDiskFS:
						result = append(result, v)
					case IsoPathAndVolumeBsdTar:
						result = append(result, v)
					case NrgPathAndVolume:
						result = append(result, v)
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
	default:
		return result, fmt.Errorf("indirect file line %d, too many/few elements: %d", lineNumber, len(quotedString))
	}
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

func IndirectFileProcessIsoPathGoDiskFS(line string, lineNumber int) (interface{}, error) {
	var result IsoPathAndVolumeWithGoDiskFS
	re := regexp.MustCompile(`[^\s"]+|"([^"]*)"`)
	parts := re.FindAllString(line, -1)

	if parts == nil || len(parts) != 3 {
		return result, fmt.Errorf("indirect file line %d, invalid ISO line", lineNumber)
	}

	p0 := StripOptionalLeadingAndTrailingDoubleQuotes(parts[0])
	p1 := StripOptionalLeadingAndTrailingDoubleQuotes(parts[1])
	if !strings.HasSuffix(p1, "/") {
		p1 += "/"
	}
	p2 := StripOptionalLeadingAndTrailingDoubleQuotes(parts[2])

	return IsoPathAndVolumeWithGoDiskFS{
		IsoPath:    p0,
		MountPoint: p1,
		VolumeName: p2,
	}, nil
}

func IndirectFileProcessIsoPathBsdTar(line string, lineNumber int) (interface{}, error) {
	var result IsoPathAndVolumeWithGoDiskFS
	re := regexp.MustCompile(`[^\s"]+|"([^"]*)"`)
	parts := re.FindAllString(line, -1)

	if parts == nil || len(parts) != 3 {
		return result, fmt.Errorf("indirect file line %d, invalid ISO line", lineNumber)
	}

	p0 := StripOptionalLeadingAndTrailingDoubleQuotes(parts[0])
	p1 := StripOptionalLeadingAndTrailingDoubleQuotes(parts[1])
	if !strings.HasSuffix(p1, "/") {
		p1 += "/"
	}
	p2 := StripOptionalLeadingAndTrailingDoubleQuotes(parts[2])

	return IsoPathAndVolumeBsdTar{
		IsoPath:    p0,
		VolumeName: p2,
	}, nil
}

func IndirectFileProcessIsoPathNrg(line string, lineNumber int) (interface{}, error) {
	var result NrgPathAndVolume
	re := regexp.MustCompile(`[^\s"]+|"([^"]*)"`)
	parts := re.FindAllString(line, -1)
	if parts == nil || len(parts) != 2 {
		return result, fmt.Errorf("indirect file line %d: expected 2 fields (nrg-path volume-name), got %d", lineNumber, len(parts))
	}
	result.NrgPath = StripOptionalLeadingAndTrailingDoubleQuotes(parts[0])
	result.VolumeName = StripOptionalLeadingAndTrailingDoubleQuotes(parts[1])
	return result, nil
}

// The index HTML files written to the DVDs are almost all in one of two (similar) formats.
// This function parses any such HTML file to produce a list of files that the index HTML links to
// and the associated part number and title recorded in the index HTML.
// If required then an MD5 checksum is generated and PDF metadata is extracted and recorded.
func ParseIndexHtml(fsys fs.FS, filename string, volume string, root string, fileExceptions *FileHandlingExceptions, md5Store *persistentstore.Store[string, string], programFlags ProgamFlags) map[string]Document {
	bytes, err := fs.ReadFile(fsys, filename)
	if err != nil {
		log.Fatalf("Error reading %s: %v", filename, err)
	}

	currentDir := path.Dir(filename)
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
	matches := re.FindAllStringSubmatch(string(bytes), -1)

	for _, match := range matches {
		linkPath := match[1]
		partNumber := strings.TrimSpace(match[2])
		title := TidyDocumentTitle(match[3])

		// 1. Check for truly-missing-file directive
		isKnownMissing := false
		for _, m := range fileExceptions.MissingFiles {
			if strings.EqualFold(m.Filepath, linkPath) {
				isKnownMissing = true
				break
			}
		}
		if isKnownMissing {
			continue
		}

		// 2. Resolve Path (Context-Aware for ../ and Fallback for root-relative)
		targetPath := path.Join(currentDir, linkPath)
		resolvedPath, err := resolvePathCaseInsensitive(fsys, targetPath)
		if err != nil {
			resolvedPath, err = resolvePathCaseInsensitive(fsys, linkPath)
		}

		// 3. Check for incorrect-filepath directive
		if err != nil {
			foundEx := false
			for _, ex := range fileExceptions.FileSubstitutes {
				if strings.EqualFold(ex.MistypedFilepath, linkPath) {
					resolvedPath, err = resolvePathCaseInsensitive(fsys, ex.ActualFilepath)
					if err == nil {
						foundEx = true
						break
					}
				}
			}
			if !foundEx {
				debug.PrintStack()
				log.Printf("MISSING file: %s in Volume %s", linkPath, volume)
				continue
			}
		}

		// Collision detection: track which original HTML path resolved to which final path.
		// Issue a warning if a truncated path matches an original (untruncated) path.
		if fileExceptions.ResolvedPathMap == nil {
			fileExceptions.ResolvedPathMap = make(map[string]string)
		}
		keyPath := volume + "/" + resolvedPath
		if prev, ok := fileExceptions.ResolvedPathMap[keyPath]; ok && prev != linkPath {
			log.Printf("WARNING: Collision in volume %s: resolved path %q already used for %q, now also for %q. Data may be incorrect.",
				volume, resolvedPath, prev, linkPath)
		} else {
			fileExceptions.ResolvedPathMap[keyPath] = linkPath
		}

		// 4. Build the Document object
		md5Sum, _ := CalculateMd5Sum(fsys, volume+"//"+resolvedPath, resolvedPath, md5Store, programFlags.Verbose)
		docPath := "file:///" + volume + "/" + resolvedPath
		newDocument := BuildNewLocalDocument(fsys, title, partNumber, resolvedPath, docPath, md5Sum, programFlags.ReadEXIF)
		newDocument.Collection = "local:" + volume

		key := md5Sum
		if key == "" {
			key = partNumber + "~" + newDocument.Format
		}

		// If a duplicate is found, keep the previous entry
		if _, ok := documentsMap[key]; ok {
			// If filepaths differ, it's a true duplicate (same content, different location)
			if newDocument.Filepath != documentsMap[key].Filepath {
				previousFilePath := documentsMap[key].Filepath
				// Create the specialized key seen in the YAML
				newKey := key + "DUPLICATE" + strings.Replace(previousFilePath, "/", "_", 20)
				documentsMap[newKey] = newDocument
			}
			// If filepaths are the same, we silently skip (same file linked twice)
		} else {
			documentsMap[key] = newDocument
		}
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
func BuildNewLocalDocument(fsys fs.FS, title string, partNum string, filePath string, documentPath string, md5Checksum string, readExif bool) Document {
	// Use fs.Stat for internal ISO/Virtual paths
	filestats, err := fs.Stat(fsys, filePath)
	if err != nil {
		log.Fatal(err)
	}

	pdfMetadata := pdfmetadata.PdfMetadata{}
	if readExif && strings.ToUpper(path.Ext(filePath)) == ".PDF" {
		pdfMetadata = pdfmetadata.ExtractPdfMetadataFromFS(fsys, filePath)
	}

	var newDocument Document
	newDocument.Format, err = DetermineFileFormat(filePath)
	if err != nil {
		// Log a warning and skip the file
		fmt.Printf("Warning: Skipping file %s due to %v\n", filePath, err)
		newDocument.Format = "UNKNOWN-DUE-TO-ERROR"
	}
	newDocument.Size = filestats.Size()
	newDocument.Md5 = md5Checksum
	newDocument.Title = strings.TrimSuffix(strings.TrimSpace(title), "\n")
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

func DetermineFileFormat(filename string) (string, error) {
	filetype := strings.TrimPrefix(strings.ToUpper(path.Ext(filename)), ".")
	if filetype == "HTM" {
		filetype = "HTML"
	}
	if filetype == "JPE" {
		filetype = "JPEG"
	}

	for _, entry := range KnownFileTypes {
		if entry == filetype {
			return filetype, nil
		}
	}
	return "", fmt.Errorf("unknown filetype: %s", filetype)
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
	title = strings.Join(strings.Fields(title), " ")
	re := regexp.MustCompile(`\s*<BR>(?:\s*<BR>\s*)*\s*`)
	title = re.ReplaceAllString(title, ". ")
	return title
}

// Return the MD5 sum for the specified file.
// Start by looking up the filename (path) in the cache and return a pre-computed MD5 sum if found.
// Otherwise, compute the MD5 sum, add the entry to the cache, mark the cache as dirty and return the computed MD5 sum.
func CalculateMd5Sum(fsys fs.FS, filenameInCache string, relativePath string, md5Store *persistentstore.Store[string, string], verbose bool) (string, error) {
	// Lookup the filename (path) in the cache; if found report that as the MD5 sum
	if md5, found := md5Store.Lookup(filenameInCache); found {
		return md5, nil
	}

	// The filename (path) is not in the cache.
	// Generate the MD5 sum, add the value to the cache and mark the cache as Dirty
	// Use fs.ReadFile for virtual filesystem compatibility
	fileBytes, err := fs.ReadFile(fsys, relativePath)
	if err != nil {
		return "", err
	}
	md5Hash := md5.Sum(fileBytes)
	md5Checksum := hex.EncodeToString(md5Hash[:])
	md5Store.Update(filenameInCache, md5Checksum)
	fmt.Printf("MD5 Store: wrote %s for [%s] (relative path %s)\n", md5Checksum, filenameInCache, relativePath)
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
	}
	return result
}

// resolvePathCaseInsensitive finds the actual case-correct path for a given name within fsys.
// It handles "." and ".." path components correctly.
func resolvePathCaseInsensitive(fsys fs.FS, name string) (string, error) {
	// The original code did the unescaping because it might have been fed a link
	// that needed to be unescaped. In fact I've not produced any such links, and this
	// is actually harmful in the case of some filenames.

	// I am leaving the commented out code along with this warning in case future-me wants to bring it back!

	// 1. Handle URL encoding (e.g. %20 -> space)
	// unescaped, err := url.QueryUnescape(name)
	// if err == nil {
	// 	name = unescaped
	// }

	// Standardise slashes and remove leading slashes (io/fs paths are relative to root)
	name = strings.ReplaceAll(name, "\\", "/")
	name = strings.TrimPrefix(name, "/")
	name = path.Clean(name)

	if name == "." || name == "" {
		return ".", nil
	}

	parts := strings.Split(name, "/")
	current := "."

	for idx, part := range parts {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			// Move up one directory
			if current == "." {
				// Cannot go above root; keep as "."
				continue
			}
			current = path.Dir(current)
			continue
		}

		// Normal component: find case-insensitive match in current directory
		entries, err := fs.ReadDir(fsys, current)
		if err != nil {
			return "", err
		}

		found := false
		for _, entry := range entries {
			if strings.EqualFold(entry.Name(), part) {
				current = path.Join(current, entry.Name())
				found = true
				break
			}
		}

		if !found {
			// Only mangle if this is the final component
			if idx == len(parts)-1 {
				part = TruncatePathForBsdTar(part)
				for _, entry := range entries {
					if strings.EqualFold(entry.Name(), part) {
						current = path.Join(current, entry.Name())
						found = true
						break
					}
				}
			}

			if !found {
				return "", fmt.Errorf("component %s not found in %s", part, current)
			}
		}
	}
	return current, nil
}

func (fi *bsdFileInfo) Name() string       { return path.Base(fi.name) }
func (fi *bsdFileInfo) Size() int64        { return fi.size }
func (fi *bsdFileInfo) Mode() fs.FileMode  { return fi.mode }
func (fi *bsdFileInfo) ModTime() time.Time { return fi.modTime }
func (fi *bsdFileInfo) IsDir() bool        { return fi.isDir }
func (fi *bsdFileInfo) Sys() interface{}   { return nil }

// NewBsdTarFS parses "bsdtar -tvf" to build a file index.
func NewBsdTarFS(isoPath string) (*BsdTarFS, error) {
	cmd := exec.Command("bsdtar", "-tvf", isoPath)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	fsys := &BsdTarFS{
		isoPath: isoPath,
		files:   make(map[string]*bsdFileInfo),
	}

	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		// Need at least 9 fields to have perms, link, owner, group, size, month, day, time, filename
		if len(fields) < 9 {
			continue
		}
		// Size is at index 4
		size, err := strconv.ParseInt(fields[4], 10, 64)
		if err != nil {
			continue // skip lines where size is not a number
		}
		// Filename is everything from index 8 onward (may contain spaces)
		name := strings.Join(fields[8:], " ")
		isDir := fields[0][0] == 'd'
		cleanName := strings.TrimSuffix(name, "/")

		var mode fs.FileMode = 0444
		if isDir {
			mode = fs.ModeDir | 0555
		}

		info := &bsdFileInfo{
			name:    cleanName,
			size:    size,
			isDir:   isDir,
			mode:    mode,
			modTime: time.Time{}, // not used
		}

		// Store with both original and "./" prefixed forms
		fsys.files[cleanName] = info
		if !strings.HasPrefix(cleanName, "./") && cleanName != "." && cleanName != "" {
			fsys.files["./"+cleanName] = info
		} else if strings.HasPrefix(cleanName, "./") {
			fsys.files[strings.TrimPrefix(cleanName, "./")] = info
		}
	}

	// Ensure root "." exists
	if _, ok := fsys.files["."]; !ok {
		fsys.files["."] = &bsdFileInfo{
			name:  ".",
			isDir: true,
			mode:  fs.ModeDir | 0555,
		}
	}

	return fsys, scanner.Err()
}

// ReadDir implements fs.ReadDirFS.
func (fsys *BsdTarFS) ReadDir(dir string) ([]fs.DirEntry, error) {
	dir = strings.Trim(dir, "/")
	if dir == "." {
		dir = ""
	}
	var entries []fs.DirEntry
	for path, info := range fsys.files {
		// Check if path is directly inside dir
		parent := path
		if idx := strings.LastIndex(parent, "/"); idx >= 0 {
			parent = parent[:idx]
		} else {
			parent = ""
		}
		if parent != dir {
			continue
		}
		// Extract base name
		base := path
		if idx := strings.LastIndex(base, "/"); idx >= 0 {
			base = base[idx+1:]
		}
		entries = append(entries, &bsdDirEntry{
			name:  base,
			isDir: info.isDir,
			info:  info,
		})
	}
	return entries, nil
}

type bsdDirEntry struct {
	name  string
	isDir bool
	info  fs.FileInfo
}

func (e *bsdDirEntry) Name() string               { return e.name }
func (e *bsdDirEntry) IsDir() bool                { return e.isDir }
func (e *bsdDirEntry) Type() fs.FileMode          { return e.info.Mode().Type() }
func (e *bsdDirEntry) Info() (fs.FileInfo, error) { return e.info, nil }
func (f *bsdFile) Stat() (fs.FileInfo, error) {
	return f.info, nil
}

// Open implements fs.FS.
func (fsys *BsdTarFS) Open(name string) (fs.File, error) {
	// Normalise: trim leading slashes, handle "." and ""
	name = strings.TrimPrefix(name, "/")
	if name == "." || name == "" {
		name = "."
	}
	// Try as-is; if not found, try with "./" prefix
	info, ok := fsys.files[name]
	if !ok {
		// Try truncated filename
		truncated := TruncatePathForBsdTar(name)
		if truncated != name {
			info, ok = fsys.files[truncated]
			if ok {
				name = truncated
			}
		}
	}
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	if info.isDir {
		return &bsdDirFile{fsys: fsys, path: name}, nil
	}
	// Regular file: stream with bsdtar -xO
	// Use the original path as stored in the map (might have "./")
	originalPath := info.name
	cmd := exec.Command("bsdtar", "-xO", "-f", fsys.isoPath, originalPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &bsdFile{
		ReadCloser: stdout,
		cmd:        cmd,
		info:       info,
	}, nil
}

// bsdFile implements fs.File for regular files.
type bsdFile struct {
	io.ReadCloser
	cmd  *exec.Cmd
	info fs.FileInfo
}

// Stat implements fs.StatFS.
func (fsys *BsdTarFS) Stat(name string) (fs.FileInfo, error) {
	// Normalise the path like Open does
	name = strings.TrimPrefix(name, "/")
	if name == "" || name == "." {
		name = "."
	}
	// Try exact match, then with "./" prefix
	info, ok := fsys.files[name]
	if !ok && !strings.HasPrefix(name, "./") {
		info, ok = fsys.files["./"+name]
	}
	if !ok {
		// Try truncated filename
		truncated := TruncatePathForBsdTar(name)
		if truncated != name {
			info, ok = fsys.files[truncated]
			if !ok && !strings.HasPrefix(truncated, "./") {
				info, ok = fsys.files["./"+truncated]
			}
		}
	}
	if !ok {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
	}
	return info, nil
}

// bsdDirFile implements fs.File for directories.
type bsdDirFile struct {
	fsys    *BsdTarFS
	path    string // normalized path (e.g., "." for root)
	entries []fs.DirEntry
	offset  int
	closed  bool
}

// normalizeDirPath converts an empty or "." path to the canonical "." representation.
func (d *bsdDirFile) normPath() string {
	if d.path == "" || d.path == "." {
		return "."
	}
	return d.path
}

// Stat returns FileInfo for the directory.
func (d *bsdDirFile) Stat() (fs.FileInfo, error) {
	path := d.normPath()
	info, ok := d.fsys.files[path]
	if !ok {
		return nil, &fs.PathError{Op: "stat", Path: path, Err: fs.ErrNotExist}
	}
	return info, nil
}

// Read is not supported for directories.
func (d *bsdDirFile) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.normPath(), Err: fs.ErrInvalid}
}

// Close marks the directory as closed.
func (d *bsdDirFile) Close() error {
	d.closed = true
	return nil
}

// ReadDir reads the contents of the directory.
func (d *bsdDirFile) ReadDir(n int) ([]fs.DirEntry, error) {
	if d.closed {
		return nil, fs.ErrClosed
	}

	path := d.normPath()

	if d.entries == nil {
		entries, err := d.fsys.ReadDir(path)
		if err != nil {
			return nil, err
		}
		d.entries = entries
	}

	if n <= 0 {
		// Return all remaining entries
		result := d.entries[d.offset:]
		d.offset = len(d.entries)
		return result, nil
	}

	end := d.offset + n
	if end > len(d.entries) {
		end = len(d.entries)
	}
	result := d.entries[d.offset:end]
	d.offset = end
	if len(result) == 0 {
		return nil, io.EOF
	}
	return result, nil
}

// TruncatePathForBsdTar truncates the filename component of a file path so that
// the total length of the filename plus its extension (including the dot) does
// not exceed BSDTAR_FILENAME_LIMIT (64) characters. The extension is defined as the substring starting
// with the last dot in the filename. If the path has no dot, the whole filename
// is truncated to BSDTAR_FILENAME_LIMIT characters.
//
// The function splits the path into directory and base name, then splits the
// base name into a name part (everything before the last dot) and an extension
// part (the last dot and what follows). It shortens the name part to fit within
// the limit, then reassembles the path. If the original already fits, it returns
// the original path unchanged.

const BSDTAR_FILENAME_LIMIT = 64

func TruncatePathForBsdTar(originalPath string) string {
	// Split into directory and base filename
	dir, base := path.Split(originalPath)

	// Find the last dot to separate extension
	lastDot := strings.LastIndex(base, ".")
	if lastDot == -1 {
		// No extension – truncate whole base to BSDTAR_FILENAME_LIMIT chars
		if len(base) <= BSDTAR_FILENAME_LIMIT {
			return originalPath
		}
		newBase := base[:BSDTAR_FILENAME_LIMIT]
		return path.Join(dir, newBase)
	}

	namePart := base[:lastDot]
	extPart := base[lastDot:] // includes the dot

	// Total allowed length for namePart is BSDTAR_FILENAME_LIMIT - len(extPart)
	maxNameLen := BSDTAR_FILENAME_LIMIT - len(extPart)
	if maxNameLen < 0 {
		// Extension alone exceeds BSDTAR_FILENAME_LIMIT – should not happen, but keep original
		fmt.Printf("WARNING: Huge extension: %s\n", originalPath)
		return originalPath
	}

	if len(namePart) <= maxNameLen {
		return originalPath
	}

	// Truncate the name part
	newNamePart := namePart[:maxNameLen]
	newBase := newNamePart + extPart
	return path.Join(dir, newBase)
}
