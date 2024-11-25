import csv
import os
import sys
from urllib.parse import urlparse

def extract_last_path_element(url):
    # Parse the URL and get the path
    parsed_url = urlparse(url)
    # Split the path into segments and return the last segment (file name)
    lpe = os.path.basename(parsed_url.path)
    if lpe[-1] == '"':
        lpe = lpe[:-1]
    return lpe

def main(directory, in_csv_file, out_csv_file):
    if not os.path.isdir(directory):
        print(f"The specified directory '{directory}' does not exist.")
        return
    files_present = 0
    files_missing = 0

    with open(in_csv_file, mode='r') as file:
        reader = csv.DictReader(file) # reader(file)
        
        with open(out_csv_file, mode='w', newline='', encoding='utf-8') as outfile:
            writer = csv.DictWriter(outfile, fieldnames=reader.fieldnames)
            writer.writeheader()  # Write the header to the output file
            for row in reader:
                if len(row) < 3:
                    print(f"Dropping badly formed row [{row}]")
                    continue  # Skip rows that don't have enough columns
            
                url = row["URL"]  # Third entry is the URL
                file_name = extract_last_path_element(url)
                title = row["Title"]
                record = row["Record"]
                date = row["Date"]

                if record != "Doc":
                    # For anything other than a "Doc" record, write it out
                    writer.writerow(row)
                elif os.path.isfile(os.path.join(directory, file_name)):
                    # For a "Doc" record, write it out the target file exists locally
                    writer.writerow(row)
                    files_present += 1
                else:
                    print(f"File does not exist: {file_name}  check with [ curl --silent -I {url} ]")
                    files_missing += 1

    print(f"Files present = {files_present}")
    print(f"Files missing = {files_missing}")

if __name__ == "__main__":
    if len(sys.argv) != 4:
        print("Usage: python script.py <directory> <input_csv_file> <output_csv_file>")
        sys.exit(1)
    
    dir_path = sys.argv[1]
    in_csv_file = sys.argv[2]
    out_csv_file = sys.argv[3]
    print(f"dir={dir_path} csv={in_csv_file}")
    main(dir_path, in_csv_file, out_csv_file)
