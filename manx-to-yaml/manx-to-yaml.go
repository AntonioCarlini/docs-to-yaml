package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	types "docs-to-yaml/structs"
)

// This program takes the manx database dump and tries to produce a YAML file
// describing a subset of the documents contained in it.
//
// The purpose is to use the information gathered by manx (when it was at vt100.net)
// to produce a document list including document titles and MD5 checksum. That should
// allow me to scans that I have archived locally but which originated on bitsavers
// (or from somewhere that bitsavers has archived). Well, that was the plan, but
// bitsavers has altered the metadata of some of its files at various times, so I need
// a more sophisticated plan of attack. Anyway, the output is still useful for other
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
// - cannot simply split on comma ... needed proper CSV parsing
// - change COPY from single quotes to double quotes (to keep the CSV package happy)

type Document = types.Document

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
			data := strings.Split(data_text, ",")
			var pubHistory PubHistory
			pubHistory.Id, err = strconv.Atoi(data[0])
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

		key := entry.Md5
		if key == "" {
			key = pubHistory.Part
			if key == "" {
				key = pubHistory.Title
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
		newDocument.Title = pubHistory.Title
		newDocument.PubDate = pubHistory.PubDate
		newDocument.PartNum = pubHistory.Part

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

	err = ioutil.WriteFile("bin/documents.yaml", data, 0644)
	if err != nil {
		log.Fatal(err)
	}

	manxData, err := yaml.Marshal(&manxMd5Map)
	if err != nil {
		log.Fatal(err)
	}

	err = ioutil.WriteFile("bin/manx-md5.yaml", manxData, 0644)
	if err != nil {
		log.Fatal(err)
	}
}
