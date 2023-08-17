
This repository holds a number of programs (mostly in Go) to help me manage the electronic documents that I have locally.

This currently covers the collection of computer-related manuals, mostly produced by manufacturers such as DEC. It currently does not cover other documents, such as e-book and magazines which may have been acquired from the internet, although this position may change in the future.

### bitsavers-to-yaml

Takes a copy of IndexByDate.txt that has been downloaded from bitsavers, along with a file that supplies the MD5 sums for many of those files and produces a YAML file that describes the subset of DEC-related information.

### file-tree-to-yaml

Generates a YAML file that describes all files under a specific root. This should help automate producing new archive discs.

### local-to-yaml

This program examines a specified set of directories that contain copies of CD-R and DVR-R copies or images that contain relevant manuals that I have collected over the years and builds up some YAML files describing the contents.
The intention is to combine this with other YAML data about various sites on the internet to help me find scans I have that are not available on any of the internet repositories that currently exist.

### manx-to-yaml

This program takes a cut-down portion of the SQL dump of the manx (a catalogue of computer manuals) database from 2010 and turns it into a YAML file describing the relevant parts of each entry. Since I managed to obtain a more up to date source of bitsavers MD5 checksums, this programme is less likely to be useful. It will still produce a set of older MD5 checksums which might be useful in verifying that some of the files I have match older versions that were available on bitsavers in the past.

