#!/bin/bash
set -e
cd "$(dirname "$0")"
if [ ! -f dist/otp4windows-amd64.exe.zip ]; then
  ./package.sh
fi
ver=$(jq -r '.RT_VERSION["#1"]["0000"].fixed.file_version' winres/winres.json)
newTag=v${ver%.*}
#gh release delete $newTag
gh release create $newTag --title "${1:-$newTag}" \
  dist/otp4windows-amd64.exe.zip \
  dist/otp4windows-arm64.exe.zip
