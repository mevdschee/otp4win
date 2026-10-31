#!/bin/bash
set -e
cd "$(dirname "$0")"
ver=$(jq -r '.RT_VERSION["#1"]["0000"].fixed.file_version' winres/winres.json)
version=${ver%.*}
msi=dist/otp4win-$version-amd64.msi
if [ ! -f $msi ]; then
  ./package.sh
fi
newTag=v$version
#gh release delete $newTag
gh release create $newTag --title "${1:-$newTag}" $msi
