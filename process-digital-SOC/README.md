
This directory holds a work in progress that I am using to properly catalogue the files I have downloaded from DEC's online SOC found on archive.org

The useful archived pages can be obtained with:

 curl https://web.archive.org/web/20000815055655/http://www.digital.com/SOHOME/SOHOMEHM.HTM > SOHOMEHM-20000815055655.HTM
 curl https://web.archive.org/web/20001013061554/http://www.digital.com/SOHOME/SOHOMEHM.HTM > SOHOMEHM-20001013061554.HTM
 curl https://web.archive.org/web/20000621091055/http://www.digital.com/SOHOME/SOHOMEHM.HTM > SOHOMEHM-20000621091055.HTM
 curl https://web.archive.org/web/20000605145520/http://www.digital.com/SOHOME/SOHOMEHM.HTM > SOHOMEHM-20000605145520.HTM
 curl https://web.archive.org/web/20000229061825/http://www.digital.com/SOHOME/SOHOMEHM.HTM > SOHOMEHM-20000229061825.HTM
 curl https://web.archive.org/web/19991129023950/http://www.digital.com/SOHOME/SOHOMEHM.HTM > SOHOMEHM-19991129023950.HTM

Each of these has a few links to PDF files and DOC files. Not all of these links work.

I managed to persuade ChatGPT to pull out the links, but overall I think it might have been quicker to write  script to do it!

I'm not 100% sure that it successfully picked up every link.

00-index.csv is a CSV file that contains one line for each link.

check-csv.py is a script that lists which entries in 00-index.csv correspond to a file in the specified directory.

build-webpage.py is a script that reads 00-index.csv and produces a web page listing every document.

DOC.gif, PNG.gif and IA.gif are images used in the web page.

This is currently very rough, but I am committing it now so that I'm less likely to lose it if I corrupt something while editing!
