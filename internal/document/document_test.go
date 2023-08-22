package document

import (
	"testing"
)

func TestDetermineDocumentFormat(t *testing.T) {
	// Test a typical PDF file: this should work
	pdf_test_path := "foo/abcd.pdf"

	format, err := DetermineDocumentFormat(pdf_test_path)
	if format != "PDF" {
		t.Fatalf(`Bad result: DetermineDocumentFormat(%s) = %q %v expected "PDF" and nil`, pdf_test_path, format, err)
	}
	if err != nil {
		t.Fatalf(`Bad error:  DetermineDocumentFormat(%s) = %q %v expected "PDF" and nil`, pdf_test_path, format, err)
	}

	// Test a file type that should not be recognised
	unk_test_path := "this_file_has.an_unrecognised_FILETYPE"
	format, err = DetermineDocumentFormat(unk_test_path)
	if format != "???" {
		t.Fatalf(`Bad result: DetermineDocumentFormat(%s) = %q %v expected "???" and ¬nil`, unk_test_path, format, err)
	}
	if err == nil {
		t.Fatalf(`Bad error:  DetermineDocumentFormat(%s) = %q %v expected "???" and ¬nil`, unk_test_path, format, err)
	}
}

func TestDetermineDocumentPropertiesFromPath(t *testing.T) {
	var doc Document
	unsetPartNum := "MADE-UP-PN"
	unsetPubDate := "1758-11-04"

	path := "/path/path/bad-part-num_Title_Text_No_Date.pdf"
	doc = DetermineDocumentPropertiesFromPath(path, false)
	if (doc.PartNum != unsetPartNum) || (doc.PubDate != unsetPubDate) || (doc.Title != "bad-part-num Title Text No Date") {
		t.Fatalf(`DetermineDocumentPropertiesFromPath(%s) failed, PN=%s Date=%s Title=%s`, path, doc.PartNum, doc.PubDate, doc.Title)
	}

	path = "/path/path/EK-ABCDE-AA-001_Title_Text_No_Date.pdf"
	doc = DetermineDocumentPropertiesFromPath(path, false)
	if (doc.PartNum != "EK-ABCDE-AA-001") || (doc.PubDate != unsetPubDate || (doc.Title != "Title Text No Date")) {
		t.Fatalf(`DetermineDocumentPropertiesFromPath(%s) failed, PN=%s Date=%s Title=%s`, path, doc.PartNum, doc.PubDate, doc.Title)
	}

	path = "/path/path/EK-ABCDE-AA-001_Title_Text_Mar83.pdf"
	doc = DetermineDocumentPropertiesFromPath(path, false)
	if (doc.PartNum != "EK-ABCDE-AA-001") || (doc.PubDate != "1983-03" || (doc.Title != "Title Text")) {
		t.Fatalf(`DetermineDocumentPropertiesFromPath(%s) failed, PN=%s Date=%s Title=%s`, path, doc.PartNum, doc.PubDate, doc.Title)
	}

	path = "/path/path/Title_Text_Mar83.pdf"
	doc = DetermineDocumentPropertiesFromPath(path, false)
	if (doc.PartNum != unsetPartNum) || (doc.PubDate != "1983-03" || (doc.Title != "Title Text")) {
		t.Fatalf(`DetermineDocumentPropertiesFromPath(%s) failed, PN=%s Date=%s Title=%s`, path, doc.PartNum, doc.PubDate, doc.Title)
	}
}

func TestBuildKeyFromDocument(t *testing.T) {
	var doc Document
	var key string

	setMd5 := "MY-MD5"
	setPartNum := "MY-PART-NUM"
	setTitle := "MY-TITLE"
	setFilepath := "MY-FILEPATH"

	doc.Md5 = setMd5
	doc.PartNum = setPartNum
	doc.Title = setTitle
	doc.Filepath = setFilepath

	key = BuildKeyFromDocument(doc)
	if key != setMd5 {
		t.Fatalf(`BuildKeyFromDocument(%#v) = %s  FAILED`, doc, key)
	}

	doc.Md5 = ""
	key = BuildKeyFromDocument(doc)
	if key != setPartNum {
		t.Fatalf(`BuildKeyFromDocument(%#v) = %s  FAILED`, doc, key)
	}

	doc.PartNum = ""
	key = BuildKeyFromDocument(doc)
	if key != setTitle {
		t.Fatalf(`BuildKeyFromDocument(%#v) = %s  FAILED`, doc, key)
	}

	doc.Title = ""
	key = BuildKeyFromDocument(doc)
	if key != setFilepath {
		t.Fatalf(`BuildKeyFromDocument(%#v) = %s  FAILED`, doc, key)
	}
}

func TestValidateDecPartNumber(t *testing.T) {
	validPartNumbers := []string{"EK-70C0B-TM.002", "EK-258AA-MG-003", "EK-AS800-RM.A01", "DS-0013D-TE", "AA-PCU9A-TE", "EY-0016E-DA-0002", "EY-U657E-SG.0001",
		"EK-AAAAA-AC"}

	for _, pn := range validPartNumbers {
		if !ValidateDecPartNumber(pn) {
			t.Fatalf(`ValidateDecPartNumber(%s) unexpectedly returned false\n`, pn)
		}
	}

	invalidPartNumbers := []string{"AAA-BBBBBBBB"}

	for _, pn := range invalidPartNumbers {
		if ValidateDecPartNumber(pn) {
			t.Fatalf(`ValidateDecPartNumber(%s) unexpectedly returned true`, pn)
		}
	}
}

func TestValidateDate(t *testing.T) {
	validDates := map[string]string{"May91": "1991-05x", "Jun00": "2000-06x", "1960": "1960x", "197912": "1979-12x"}

	for k, v := range validDates {
		result := ValidateDate(k)
		if result != v {
			t.Fatalf(`ValidateDate(%s) returned %s but should have returned %s`, k, result, v)
		}
	}
}

func TestSetFlags(t *testing.T) {
	var doc Document
	doc.Flags = ""

	SetFlags(doc, "?")
	if doc.Flags != "" {
		t.Fatalf(`with doc.Flags = "", document.SetFlags(doc, "?") returned flags: %s but should have been ""`, doc.Flags)
	}

	SetFlags(doc, "T")
	if doc.Flags != "T" {
		t.Fatalf(`with doc.Flags = "", document.SetFlags(doc, "T") returned flags: %s but should have been T`, doc.Flags)
	}

	SetFlags(doc, "T")
	if doc.Flags != "T" {
		t.Fatalf(`with doc.Flags = "T", document.SetFlags(doc, "T") returned flags: %s but should have been T`, doc.Flags)
	}

	SetFlags(doc, "P")
	if doc.Flags != "TP" {
		t.Fatalf(`with doc.Flags = "T", document.SetFlags(doc, "P") returned flags: %s but should have been TP`, doc.Flags)
	}
}

func TestClearFlags(t *testing.T) {
	var doc Document
	doc.Flags = "PTD"

	ClearFlags(doc, "?")
	if doc.Flags != "PTD" {
		t.Fatalf(`with doc.Flags = "PTD", document.ClearFlags(doc, "?") returned flags: %s but should have been "PTD"`, doc.Flags)
	}

	ClearFlags(doc, "T")
	if doc.Flags != "PD" {
		t.Fatalf(`with doc.Flags = "PTD", document.ClearFlags(doc, "T") returned flags: %s but should have been PD`, doc.Flags)
	}

	ClearFlags(doc, "T")
	if doc.Flags != "PD" {
		t.Fatalf(`with doc.Flags = "PD", document.ClearFlags(doc, "T") returned flags: %s but should have been PD`, doc.Flags)
	}

	doc.Flags = "PTD"
	ClearFlags(doc, "PD")
	if doc.Flags != "T" {
		t.Fatalf(`with doc.Flags = "PTD", document.ClearFlags(doc, "PD") returned flags: %s but should have been T`, doc.Flags)
	}
}
