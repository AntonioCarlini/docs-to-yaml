#!/bin/bash
# $1 is the path to use
path="${1%/}"
echo "Building ${path}/md5sums"

find "${path}" -type f -exec md5sum {} + | sed "s|${path}/||" |  sed -n '/index.yaml$\|index.csv$\|md5sums$/!p' > "${path}/md5sums"
