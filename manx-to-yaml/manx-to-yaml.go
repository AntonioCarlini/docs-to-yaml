package main

import (
	"bufio"
	"docs-to-yaml/internal/document"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// This program takes the manx database dump and tries to produce a YAML file
// describing a subset of the documents contained in it.
//
// The purpose is to use the information gathered by manx (when it was at vt100.net)
// to produce a document list including document titles and MD5 checksum. That should
// allow me to find scans that I have archived locally but which originated on bitsavers
// (or from somewhere that bitsavers has archived). Well, that was the plan, but
// bitsavers has altered the metadata of some of its files at various times, so I need
// a more sophisticated plan of attack. Anyway, the output may still be useful for other
// purposes in the future.
//
// For a given document, record at least:
// * Title
// * Part number
// * Publisher (DEC, Emulex, Dilog)
// * Date of publication
// * MD5
//
// Also produce a pair of URL => MD5
//

// At the moment the table filenames are hard coded as the only publically available SQL dump is from
// 2010, when manx moved to its current maintainer. As the full SQL dump is too unwieldy (160MB),
// the necessary tables have been extracted (the largest being only 4.2MB).

// Table 'COPY' lists every document, listing URL, MD5, pub-id
// Table 'PUB' lists publications, but really serves as a pointer to PUB_HISTORY
// Table 'PUB_HISTORY' lists company, part #, publication date, title

// Issues:
// - change COPY from single quotes to double quotes (to keep the CSV package happy)

const (
	// The PUB_HISTORY dump of Manx from 2010 has 24 fields.
	// This will never change, so define it here to allow a sanity check in the code.
	PUB_HISTORY_SQL_DUMP_NUM_FIELDS = 24
)

type Document = document.Document

// The manx SQL 'COPY' table lists each copy of  a document
type Copy struct {
	Id           int
	Pub          int
	Format       string // e.g. PDF, HTML etc.
	Site         int
	Url          string
	Notes        string
	Size         int64
	Md5          string
	Credits      string
	Amend_serial int
}

// The manx SQL 'PUB' table lists each publication
// For our purposes each entry is simply the ID of a PUB_HISTORY.
// It is kept as a struct because it is easy to do and in case other fields prove to be useful later.
type Pub struct {
	Id               int
	Active           bool
	PubHistory       int
	HasOnlineCopies  bool
	HasOfflineCopies bool
	HasTOC           bool
	IsSuperseded     bool
}

// The manx SQL 'PUBHISTORY' table lists each publication.
// It includes title, part number and company, amongst other details
type PubHistory struct {
	Id           int
	Active       bool
	Created      string
	EditedBy     int
	PubId        int
	PubType      byte
	Company      int
	Part         string
	AltPart      string
	Revision     string
	PubDate      string
	Title        string
	Keywords     string
	Notes        string
	Class        string
	MatchPart    string
	MatchAltPart string
	SortPart     string
	Abstract     string
	OcrFile      string
	CoverImage   string
	Language     string
	AmendPub     int
	AmendSerial  int
}

func parseManxCopyTable(filename string) []Copy {
	file, err := os.Open(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	var copyTable []Copy

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "INSERT INTO `COPY` VALUE") {
			// Extract the part between "(" and ");", discarding the rest
			start := strings.Index(line, "(") + 1
			end := strings.LastIndex(line, ");")
			data_text := line[start:end]
			r := csv.NewReader(strings.NewReader(data_text))
			r.LazyQuotes = true
			data, err := r.Read()
			if err != nil {
				fmt.Println("Problem with line: [", data_text, "]")
				log.Fatal(err)
			}

			// data := strings.Split(data_text, ",")
			var copy Copy
			copy.Id, err = strconv.Atoi(data[0])
			copy.Pub, err = strconv.Atoi(data[1])
			copy.Format = data[2]
			// copy.Site = data[3]
			copy.Url = data[4]
			copy.Notes = data[5]
			copy.Size, err = strconv.ParseInt(data[6], 10, 0)
			if data[7] == "NULL" {
				copy.Md5 = ""
			} else {
				copy.Md5 = data[7]
			}
			copy.Credits = data[8]
			// copy.Amend_serial = data[9]
			if len(copy.Md5) != 32 && copy.Md5 != "" {
				fmt.Println("Odd MD5 in COPY ", copy.Id, " = ", copy)
			}
			// fmt.Println("Data: pub=", copy.Pub, "URL=", copy.Url, "MD5=", copy.Md5, "size=", copy.Size)
			copyTable = append(copyTable, copy)
		}
	}
	return copyTable
}

func parseManxPubTable(filename string) map[int]Pub {
	file, err := os.Open(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	pubMap := make(map[int]Pub)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "INSERT INTO `PUB` VALUES") {
			// Extract the part between "(" and ");", discarding the rest
			start := strings.Index(line, "(") + 1
			end := strings.LastIndex(line, ");")
			data_text := line[start:end]
			data := strings.Split(data_text, ",")
			var pub Pub
			pub.Id, err = strconv.Atoi(data[0])
			// pub.Active, err = strconv.Atoi(data[1])
			pub.PubHistory, err = strconv.Atoi(data[2])
			// pub.HasOnlineCopies = data[3]
			// pub.HasOfflineCopies = data[4]
			// pub.HasTOC = data[5]
			// pub.IsSuperseded = data[6]
			pubMap[pub.Id] = pub
		}
	}
	return pubMap
}

func parseManxPubHistoryTable(filename string) map[int]PubHistory {
	file, err := os.Open(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	pubHistoryMap := make(map[int]PubHistory)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "INSERT INTO `PUBHISTORY` VALUES") {
			// Extract the part between "(" and ");", discarding the rest
			start := strings.Index(line, "(") + 1
			end := strings.LastIndex(line, ");")
			data_text := line[start:end]

			//r := csv.NewReader(bytes.NewReader([]byte(data_text)))
			//r.Comma = ','
			// r.LazyQuotes = true
			//r.Quote = '\'' // Use single quotes as the quote character

			// Manually split by commas, handling quoted values
			// encoding/csv won't handle any quoting character other than a double quote
			data := []string{}
			field := ""
			inQuotes := false
			previousChar := '?'
			for _, char := range data_text {
				if char == '\'' && previousChar != '\\' {
					// Seeing a quote switches into and out of quote mode
					// (unless this is an escaped single quote: \')
					inQuotes = !inQuotes
				} else if char == ',' && !inQuotes {
					// If a ',' is seen outside of quotes, this is the end of a field
					data = append(data, field)
					field = ""
				} else {
					// Otherwise append this character to the current field
					field += string(char)
					previousChar = char
				}
			}

			// Add last field
			if field != "" {
				data = append(data, field)
			}

			// Output the parsed values

			if len(data) != PUB_HISTORY_SQL_DUMP_NUM_FIELDS {
				fmt.Println("Read: [" + data_text + "]")
				fmt.Println("Parsed values (len=" + strconv.Itoa(len(data)) + "):")
				for i, field := range data {
					fmt.Printf("%d: %s\n", i+1, field)
				}
			}

			var pubHistory PubHistory
			pubHistory.Id, err = strconv.Atoi(data[0])
			if err != nil {
				fmt.Println("Error converting number ["+data[0]+"] in line: ["+data_text+"]", err)
				continue
			}
			// pubHistory.Active, err = strconv.Atoi(data[1])
			pubHistory.Created = data[2]
			// pubHistory.EditedBy, err = strconv.Atoi(data[3])
			// pubHistory.PubId, err = strconv.Atoi(data[4])
			// pubHistory.PubType, err = strconv.Atoi(data[5])
			// pubHistory.Company, err = strconv.Atoi(data[6])
			pubHistory.Part = data[7]
			if pubHistory.Part == "NULL" {
				pubHistory.Part = ""
			}
			pubHistory.AltPart = data[8]
			pubHistory.Revision = data[9]
			pubHistory.PubDate = data[10]
			if pubHistory.PubDate == "NULL" {
				pubHistory.PubDate = ""
			}
			pubHistory.Title = data[11]
			pubHistory.Keywords = data[12]
			pubHistory.Notes = data[13]
			pubHistory.Class = data[14]
			pubHistory.MatchPart = data[15]
			pubHistory.MatchAltPart = data[16]
			pubHistory.SortPart = data[17]
			pubHistory.Abstract = data[18]
			pubHistory.OcrFile = data[19]
			pubHistory.CoverImage = data[20]
			pubHistory.Language = data[21]
			// pubHistory.AmendPub, err = strconv.Atoi(data[22])
			// pubHistory.AmendSerial, err = strconv.Atoi(data[23])
			pubHistoryMap[pubHistory.Id] = pubHistory
		}
	}
	return pubHistoryMap
}

func main() {
	copyTable := parseManxCopyTable("data/manx-mysql-dump-20100609-COPY")
	fmt.Println("COPY size", len(copyTable))
	pubMap := parseManxPubTable("data/manx-mysql-dump-20100609-PUB")
	fmt.Println("PUB size", len(pubMap))
	pubHistoryMap := parseManxPubHistoryTable("data/manx-mysql-dump-20100609-PUB_HISTORY")
	fmt.Println("PUBHISTORY size", len(pubHistoryMap))

	output_yaml_file := flag.String("yaml-output", "", "filepath of the output file to hold the generated yaml")
	output_md5_file := flag.String("md5-output", "", "filepath of the output file to hold the generated yaml")

	flag.Parse()

	fatal_error_seen := false

	if *output_yaml_file == "" {
		log.Print("--yaml-output is mandatory - specify an output YAML file")
		fatal_error_seen = true
	}

	if fatal_error_seen {
		log.Fatal("Unable to continue because of one or more fatal errors")
	}

	// We want to produce a map of unique documents.
	// If an MD5 is present, that's enough to guarantee uniqueness.
	// If no MD5 is present, use the part number
	// If no part number is present, use the title
	// Look for duplicate (non-empty) MD5 values
	documentsMap := make(map[string]Document)

	// Build a map of MD5 to URL
	manxMd5Map := make(map[string]string)

	for _, entry := range copyTable {
		var pubHistory PubHistory
		var pub Pub
		var ok bool
		// var
		if pub, ok = pubMap[entry.Pub]; ok {
			if pubHistory, ok = pubHistoryMap[pub.PubHistory]; !ok {
				fmt.Println("Cannot find PUBHISTORY", pub.PubHistory, " in PUB", entry.Pub, " in COPY", entry.Id)
				continue
			}
		} else {
			fmt.Println("Cannot find PUB ", entry.Pub, " in COPY", entry.Id)
			continue
		}

		title := StripOptionalLeadingAndTrailingSingleQuotes(pubHistory.Title)
		partNum := StripOptionalLeadingAndTrailingSingleQuotes(pubHistory.Part)
		publicUrl := StripOptionalLeadingAndTrailingSingleQuotes(entry.Url)

		key := entry.Md5
		if key == "" {
			key = partNum
			if key == "" {
				key = title
			}
		}

		// Eliminate any duplicate entries
		if _, ok := documentsMap[key]; ok {
			//fmt.Println("Repeated key", key, " for COPY", entry.Id, "PUB_HISTORY", pubHistory.Id)
			continue
		}

		// Build a document structure and add it to the map
		var newDocument Document
		newDocument.Format = entry.Format
		newDocument.Size = entry.Size
		newDocument.Md5 = entry.Md5
		newDocument.Title = title
		newDocument.PubDate = pubHistory.PubDate
		newDocument.PartNum = partNum
		newDocument.PublicUrl = publicUrl

		documentsMap[key] = newDocument
		if entry.Md5 != "" {
			manxMd5Map[entry.Md5] = entry.Url
		}

	}
	fmt.Println("Documents size", len(documentsMap))

	//for _, document := range documentsMap {
	//	fmt.Println("Part", document.PartNum, "Title", document.Title)
	//}

	data, err := yaml.Marshal(&documentsMap)
	if err != nil {
		log.Fatal(err)
	}

	err = os.WriteFile(*output_yaml_file, data, 0644)
	if err != nil {
		log.Fatal(err)
	}

	manxData, err := yaml.Marshal(&manxMd5Map)
	if err != nil {
		log.Fatal(err)
	}

	// The output MD5 file is optional
	if *output_md5_file != "" {
		err = os.WriteFile(*output_md5_file, manxData, 0644)
		if err != nil {
			log.Fatal(err)
		}
	}

}

// Helper function to remove leading and trailing single quotes, if present.
// Otherwise returns the original string untouched.
// The SQL dump format seems to write out a string with spaces surrounded by single quotes.
func StripOptionalLeadingAndTrailingSingleQuotes(candidate string) string {
	if len(candidate) == 0 {
		return candidate
	}
	result := candidate
	if (result[0] == '\'') && (result[len(result)-1] == '\'') {
		result = result[1 : len(result)-1]
		// fmt.Printf("removed quotes from: [%s]\n", candidate)
		// fmt.Printf("result is          :  [%s]\n", result)
	}
	return result
}
