
This repository holds a number of programs (mostly in Go) to help me manage the electronic documents that I have locally.

This currently covers the collection of computer-related manuals, mostly produced by manufacturers such as DEC. It currently does not cover other documents, such as e-book and magazines which may have been acquired from the internet, although this position may change in the future.

## Overview ##

| Directory                      | Notes
|--------------------------------|-------------------------------------------------------------------------------------------|
| bin/                           | output files
| bitsavers-to-yaml/             | produces bin/bitsavers.yaml, describing documents on bitsavers
| csv/                           | ?
| data/                          | input files
| file-tree-to-yaml/             | ?
| find-locally-unique/           | ?
| first-pass/                    | ?
| internal/                      | internal go helpers
| local-archive-to-yaml/         | ?
| manx-to-yaml/                  | produces bin/manx.yaml, describing historic data from manx
| process-digital-SOC/           | helpers to produce CSV files for SOC files found on www.digital.com via archive.org
| vaxhaven-to-yaml/              | produces bin/vaxhaven.yaml, describing documents on bitsavers

## Infrastructure Files ##

### Inputs ###

_bin/filesize.store_ is a YAML file that lists URL or local file path against filesize. It is intended to act as a cache of file size data that would otherwise have to be read from either a local filesystem or from a website.

_bin/md5.store_ is a YAML file that lists URL or local file path against that file's MD5 checksum. It is intended to act as a cache of MD5 checksums and speeds up processing by avoiding re-computing MD5 checksums unless absolutely necessary.

_data/bitsavers-IndexByDate.txt_ is taken unchanged from https://bitsavers.org/pdf/IndexByDate.txt (or any official mirror). It should be re-fetched whenever significant new data is available.

_data/VaxHaven.txt_ is a of manually concatenated web pages from the www.vaxhaven.com website. The intention is to parse this accumulated HTML data to produce a list of documents found on that website.

### Outputs ###

_bin/bitsavers.yaml_ is a collection of YAML that describes documents found on the bitsavers website.

_bin/vaxhaven.yaml_ is a collection of YAML that describes documents found on the www.vaxhaven.com website.


## YAML Producers ##

### bitsavers-to-yaml

This program produces a YAML file that describes each DEC-related document found on http://www.bitsavers.org.

It takes a copy of _data/bitsavers-IndexByDate.txt_ that has been downloaded from bitsavers, along with a file that supplies the MD5 sums for many of those files and produces _bin/bitsavers.yaml_, a YAML file that describes the relevant documents.

### file-tree-to-yaml

Generates a YAML file that describes all files under a specific root. This should help automate producing new archive discs.

### local-archive-to-yaml

This program examines a specified set of directories that contain copies of CD-R and DVR-R copies or images that contain relevant manuals that I have collected over the years and builds up some YAML files describing the contents.
The intention is to combine this with other YAML data about various sites on the internet to help me find scans I have that are not available on any of the internet repositories that currently exist.

### manx-to-yaml

This program takes a cut-down portion of the SQL dump of the manx (a catalogue of computer manuals) database from 2010 and turns it into a YAML file describing the relevant parts of each entry. Since I managed to obtain a more up to date source of bitsavers MD5 checksums, this programme is less likely to be useful. It will still produce a set of older MD5 checksums which might be useful in verifying that some of the files I have match older versions that were available on bitsavers in the past.

### vaxhaven-to-yaml ###

This program produces a YAML file that describes each document found on http://www.vaxhaven.com.

It reads _data/VaxHaven.txt_, processes it and outputs _bin/vaxhaven.yaml_.  
_bin/filesize.store_ may be updated.  
_bin/md5.store_ neither used nor updated.

### yaml-to-csv ###

This program takes a set of YAML files containing document details and produces a CSV file that aggregates all those documents.  
Not all of the data for each document is written, but title, part number and location information are included.

