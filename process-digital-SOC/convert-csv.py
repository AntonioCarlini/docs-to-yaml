import csv
import sys
from datetime import datetime
import os

def parse_date(date_str):
    # Parse and format date to YYYY-MM-DD format
    for fmt in ("%d %B %Y", "%d %b %Y", "%Y-%m-%d"):  # Include formats you wish
        try:
            return datetime.strptime(date_str, fmt).strftime("%Y-%m-%d")
        except ValueError:
            continue
    return ""  # Return empty if parsing failed

def get_filename_from_path(path):
    return os.path.basename(path)

def process_csv(input_file):
    reader = csv.reader(input_file)
    # Skip header
    next(reader)
    
    output_lines = []

    print('"Record","Title","File","URL","Date","Part Number","Options"')
    
    for row in reader:
        if len(row) < 5:  # Ensure there are enough columns
            continue

        record_type = row[0].strip('"')  # Handling potential surrounding quotes
        title = row[1].strip('"')
        link = row[2].strip('"')
        date = parse_date(row[3].strip('"'))
        page_count = row[4].strip('"')

        if record_type == "Section":
            output_lines.append(["Section", title, "", link, date, "", "", ""])

        elif record_type == "Subsection":
            output_lines.append(["Subsection", title, "", link, date, "", "", ""])

        else:
            filename = get_filename_from_path(link.strip('"'))  # Original path for Filename
            output_lines.append(
                [
                    "Doc",
                    title,
                    filename,
                    link,
                    date,
                    "",
                    f"containing-page='{record_type}' page-count='{page_count}'"
                ]
            )

    # Write to standard output as CSV
    writer = csv.writer(sys.stdout)
    writer.writerows(output_lines)

if __name__ == "__main__":
    with sys.stdin as input_file:
        process_csv(input_file)
