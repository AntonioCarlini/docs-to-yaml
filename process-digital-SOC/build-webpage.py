# TODO
# note missing local files
# allow for checking of IA urls ...  if available on web but not local, then complain loudly
# if all four missing, then not as a comment in the HTML
# Add "scrolling" line or table rows?
# right justofy date and make field wider
# make title field wider too

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

# source-file, title, url, date, pagecount
# for each entry, use the title as an index into a dictionary to find a matching object, create one if necessary
# if one already exists add the PDF or DOC, checking that there isn't already one present
# Once all done check that each entry has exactly one PDF and one DOC
# Now produce the web page; order the elemnents in the order that they are seen in the CSV file

class Document:
  def __init__(self, title, source, date, pages):
      self.title = title
      self.pdf_file = ""
      self.doc_file = ""
      self.pdf_url = ""
      self.doc_url = ""
      self.source = source
      self.date = date
      self.page_count = pages
      
def main(csv_file, directory):
    if not os.path.isdir(directory):
        print(f"The specified directory '{directory}' does not exist.")
        return

    docs = {}
    
    with open(csv_file, mode='r') as file:
        reader = csv.reader(file)
        
        for row in reader:
            if len(row) < 4:
                continue  # Skip rows that don't have enough columns
            if row[0] == "PageSourceFile":
                continue  # Skip the first row
            
            url = row[2]  # Third entry is the URL
            file_name = extract_last_path_element(url)
            source = row[0]
            title = row[1]
            date = row[3]
            pages = row[4]
            
            if (source == "Section") or (source == "Subsection"):
                heading = True
                dict_key = title + url
            else:
                heading = False
                dict_key = title + source

            element = docs.get(dict_key)
            if element == None:
                element = Document(title, source, date, pages)
                docs[dict_key] = element

            docs[dict_key].source = source
            docs[dict_key].date = date
            docs[dict_key].page_count = pages
            if heading:
                docs[dict_key].pdf_url = url
                docs[dict_key].doc_url = url
                continue

            _, extension = os.path.splitext(file_name)
            extension = extension.upper()
            if extension == ".PDF":
                docs[dict_key].pdf_file = file_name
                docs[dict_key].pdf_url = url
            elif extension == ".DOC":
                docs[dict_key].doc_file = file_name
                docs[dict_key].doc_url = url
            else:
                print(f"Unknown filetype in [{url}]")

    # Perform some minimal checks
    for k in docs:
        if (docs[k].source == "Section") or (docs[k].source == "Subsection"):
            continue
        if docs[k].pdf_file == "":
            print(f"Missing PDF file for {docs[k].title} ({docs[k].source})")
        if docs[k].pdf_url == "":
            print(f"Missing PDF url  for {docs[k].title}")
        if docs[k].doc_file == "":
            print(f"Missing DOC file for {docs[k].title}")
        if docs[k].doc_url == "":
            print(f"Missing DOC url  for {docs[k].title}")

#    print("all docs")
#    for k in docs:
#        print(f"{k} => PDF: {docs[k].pdf_file} DOC: {docs[k].doc_file}")


    print('<!DOCTYPE HTML PUBLIC "-//IETF//DTD HTML//EN">')
    print('<html>')
    print('<head>')
    print('<title> DEC Online Systems and Options Catalogue </title>')
    print('<style type="text/css">')
    print('table, td {border:1px solid #000; margin:10px}')
    print('tr td:nth-child(1) { padding-right: 7pt; padding-left: 3pt; }')
    print('tr td:nth-child(3) { padding-right: 3pt; padding-left: 17pt; }')
    print('tr td:nth-child(4) { padding-right: 3pt; padding-left: 17pt; }')
    print('.container { display: flex; justify-content: space-between; }')
    print('</style>')
    print('</head>')
    print('<body bgcolor=#FFFFFF text=#000000>')
    print('')
    print('')
    print('<h1> DEC Online Systems and Options Catalogue </h1>')
    print('<p><hr><p>')
    print('')
    print('<table border=0>')
    print('<tr> <td> &nbsp; <td> &nbsp; </tr>')
    print('')
    last_section_seen = ""
    for k in docs:
        if docs[k].source == "Section":
            last_section_seen = docs[k].title
            print('<tr> <td colspan=4> &nbsp; </td> </tr>')
            print(f"<tr align=left bgcolor=\"d2b48c\"> <font color=\"800000\"> <th colspan=3>")
            print(f"<a id=\"{last_section_seen}\" href=\"{docs[k].pdf_url}\"> {docs[k].title} </a> </th>")
            print(f"<th> (SOC {docs[k].date}) </th>")
            print('</tr>')
            print('<tr> <td colspan=4> &nbsp; </td> </tr>')
        elif docs[k].source == "Subsection":
            print('<tr> <td colspan=4> &nbsp; </td> </tr>')
            print(f"<tr align=left> <th colspan=4 bgcolor=\"f1c40f\"> <a id=\"{docs[k].title}\">")
            print(f"<font color=\"800000\"> {last_section_seen}: {docs[k].title} </a> </tr>")
            print('<tr> <td colspan=4> &nbsp; </td> </tr>')
        else:
            print(f"<tr valign=top>")
            print(f"  <td> {docs[k].title}")
            if docs[k].pdf_file != "":
                print(f"  <td> <a href=\"{docs[k].pdf_file}\"> <img src=\"PDF.gif\" alt=\"PDF icon\" style=\"width:42px;height:42px;\"> </a>")
                print(f"       <a href=\"{docs[k].pdf_url}\">  <img src=\"IA.gif\"  alt=\"IA icon\"  style=\"width:42px;height:42px;\"> </a>")
            else:
                print(f"  <td> PDF missing)")
            if docs[k].doc_file != "":
                print(f"  <td> <a href=\"{docs[k].doc_file}\"> <img src=\"DOC.gif\" alt=\"DOC icon\" style=\"width:42px;height:42px;\"> </a>")
                print(f"       <a href=\"{docs[k].doc_url}\">  <img src=\"IA.gif\"  alt=\"IA icon\"  style=\"width:42px;height:42px;\"> </a>")
            else:
                print(f"  <td> DOC missing)")
            print(f"  <td> {docs[k].date}")
            print(f"  </td> </tr>")
    print('</table>')
    print('</body>')
    print('</html>')

if __name__ == "__main__":
    if len(sys.argv) != 3:
        print("Usage: python script.py <csv_file> <directory>")
        sys.exit(1)
    
    csv_file_path = sys.argv[2]
    dir_path = sys.argv[1]
    main(csv_file_path, dir_path)
