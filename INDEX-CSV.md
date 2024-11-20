This document describes a format to represent data for documents and other files that are to be archived.

### Motivation

As I have archived documents, I've generally included an index.html to help with navigation, and also put the same data in a text file and a PDF file. That works well for navigation but as I've come to try and build a compendium of all the douments I have, or to try and add those documents to the refs.info or pubs.info in this repository, I've found that processing those files is quite tricky, with quite a few edge cases. Furthermore, while perfectly good for generation of a web page, those index files lack information that could easily have been captured at the time, such as the origin of the document.

For that reason I've decided that I want to come up with a data format that will go some way to solving these issues.

At the moment this format is intended to be a simple way to provide meta data for documents that have been scanned or downloaded and are going to be archived. By keeping this data with the documents in a standard format it should hopefully mean that the work of gathering information about the documents only needs to be done once and then further processing can be performed with scripts.

### Introduction

To make it easy to find, this data will normally be stored in CSV format asa file named 00-index.csv in the top level of the directory tree to which it applies. For larger directory trees, it will be possible to have further index files spread throughout the tree and have them referenced as a tree, perhaps from a top-level 00-index.csv.

It is expected that that this CSV file will be processed by scripts to produce, for example, an HTML file (possibly split into multiple levels if there are many documents) that provides easy access to all the documents. It may also be processed to produce data for the refs.info and pubs.info files kept elsewhere in this repository.

### Requirements

The format should be simple enough that initial production is not overbudensome (to encourage production of the index in the first place) but sufficiently extensible that further information can be added in the future in those cases where it is available or necessary. Ideally there should be a versioning scheme in to allow processing of older index files as the format evolves.

Furthermore the format should allow a method to indicate structured groupings of the data, for example to allow a script to collect and group all documents of a certain kind together when producing an HTML file, for example.

Required information:

* Document title
* Relative path to file in local directory tree
* Full URL for the original document (if any)
* Document part number (if any)
* Document date (if any)


### Format

The CSV record format is as follows.

The first field is the 'Reecord Type" and its value determines the meaning of the remaining fields in the record.

#### Record Type

The 'Record type' field may have the following values:

* "Doc" : specifies a document and is described in the [Document Record](#document-record) section
* "Section": specifies a section heading and is described in the  [Section Record](#section-record) section
* "Subsection": specifies a sub-section heading and is described in the [Subsection Record](#subsection-record) section

#### Document Record

The fields in a Docuemnt record have the follwoing meanings:

| Field #  | Contents           |
|----------|--------------------|
|       1  | Record type        |
|       2  | Document title     |
|       3  | Local file path    |
|       4  | Original URL       |
|       5  | Document date      |
|       6  | Part number        |
|       7  | Options            |



#### Section Record

section TBD

#### Subsection Record

subsection TBD



