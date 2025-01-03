import argparse
import csv
import hashlib
import os
import re
import sys

"""
The original format for an index.csv file relegated the file's MD5 checksum to the 'options'
column, column #7. That was fine for files found on the web, where the MD5 checksum may not be
available, but the index.csv format is mainly intended for local archives.

So the index.csv format was updated to use column #7 for the MD5 checksum and moved the options
to column #8. As at this point only a few index.csv files existed, it was decided to upgrade
all of them to the new format and then start afresh as though the MD5 checksum had always been column #7.

There were not many CSV files to upgrade, but there were enough that writing a script to perform the
upgrade was likely to save considerable time. This is that script.

In addition, many of the existing index.csv files did not record the individual file MD5 checksums,
so that capability was added to this script to esnure that the resulting index.csv files were
as up-to-date as possible.

The script does perform some minimal sanity checks on the original index.csv to ensure that it only
converts source CSV files that are very likely to be the original (now deprecated) format.
The first row of the original index.csv file must match the expected header  exactly.
All other rows, except for completely empty rows, must have exactly the same number of columns
as the expected header.

Usage:
    python3 upgrade-csv.py --input-file OLD_FORMAT-CSV --output-file NEW-FORMAT-CSV --verbose

    If --verbose is present then informational messages are written to the terminal.
    Otherwise only error messages are written.

"""

def calculate_md5_checksum(filename):
    # Create an MD5 hash object
    md5_hash = hashlib.md5()

    # Open the file in binary mode
    with open(filename, 'rb') as file:
        # Read the file in chunks to avoid memory issues with large files
        for byte_block in iter(lambda: file.read(4096), b""):
            md5_hash.update(byte_block)

    # Return the hexadecimal representation of the MD5 checksum
    return md5_hash.hexdigest()


def process_csv(input_file, output_file, verbose):
    # Open the input CSV file

    path = os.path.dirname(input_file)
    with open(input_file, mode='r', newline='', encoding='utf-8') as infile:
        reader = csv.reader(infile)
        line_num = 1

        header = next(reader)  # Read header
        expected_old_header = ["Record", "Title", "File", "URL", "Date", "Part Number", "Options"]
        expected_columns = len(expected_old_header)
        if header != expected_old_header:
            raise("Bad header in ", input_file)

        # Open the output CSV file
        with open(output_file, mode='w', newline='', encoding='utf-8') as outfile:
            writer = csv.writer(outfile)
            pattern = r"md5='[^']*'"

            # Write the header to the output file
            new_header = ["Record", "Title", "File", "URL", "Date", "Part Number", "MD5 Checksum", "Options"]
            writer.writerow(new_header)
            
            # Process each row and write the modified row to the output file
            for row in reader:
                line_num += 1
                if verbose:
                    print("Read: ", row)

                if len(row) == 0:
                    if verbose:
                        print("Ignoring empty row")
                        continue
                elif len(row) != expected_columns:
                    print("Invalid data found in ", input_file, " on line ", line_num, "(", row, ")", file=sys.stderr)
                    print("Expected ", expected_columns, " columns but found ", len(row), file=sys.stderr)
                    raise("Fatal error found in processing")
                md5 = ''
                options = row[6]
                
                if row[0] != "Doc":
                    # Anything that is not a document record should be rewritten unchanged but with an extra blank MD5 Checksum column
                    output = row[0], row[1], row[2], row[3], row[4], row[5], md5, options
                    writer.writerow(output)  # Write the processed row to the new CSV
                    if verbose:
                        print("Wrote:", row)
                else:
                    # For a document record:
                    # (1) extract a possible md5='...' from the options
                    # (2) if no MD5 checksum found, generate one
                    if verbose:
                        print("Found options: ", options)
                    match = re.search(pattern, options)
                    if match:
                        md5 = match.group(0)
                        options = options.replace(md5, '')
                    if verbose:
                        print("Found MD5 : [", md5, "]  options=", options)
                    if md5 == '':
                        md5 = calculate_md5_checksum(path + "/" + row[2])
                    if verbose:
                        print("Calculated MD5 : [", md5, "]  options=", options)
                    output = [row[0], row[1], row[2], row[3], row[4], row[5], md5, options]
                    writer.writerow(output)  # Write the processed row to the new CSV
                    if verbose:
                        print("Wrote:", row)

def main():
    # Initialize argparse
    parser = argparse.ArgumentParser(description="Upgrade input CSV to have MD5 checksum as extra column and write as output CSV.")
    
    parser.add_argument('--input-file', type=str, help="input CSV file")
    parser.add_argument('--output-file', type=str, help="output CSV file")
    parser.add_argument('--verbose', action="store_true", help="produce verbose informational messages") 

    args = parser.parse_args()

    process_csv(args.input_file, args.output_file, args.verbose)

# Invoke main handler
if __name__ == "__main__":
    main()
