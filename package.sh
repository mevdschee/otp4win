#!/bin/bash
#
# go install github.com/tc-hib/go-winres@latest
#
# Builds dist/otp4windows-{amd64,arm64}.exe.zip. The version is read from
# winres/winres.json (file_version "major.minor.patch.build"); the build
# number is incremented on every run, like fyne package does.
set -e
cd "$(dirname "$0")"
json=winres/winres.json
ver=$(jq -r '.RT_VERSION["#1"]["0000"].fixed.file_version' $json)
version=${ver%.*}
build=$(( ${ver##*.} + 1 ))
jq --arg v "$version" --arg fv "$version.$build" '
  .RT_VERSION["#1"]["0000"].fixed.file_version = $fv |
  .RT_VERSION["#1"]["0000"].fixed.product_version = $fv |
  .RT_VERSION["#1"]["0000"].info["0409"].FileVersion = $v |
  .RT_VERSION["#1"]["0000"].info["0409"].ProductVersion = $v
' $json > $json.tmp && mv $json.tmp $json
echo "Building otp4windows $version (build $build)"
~/go/bin/go-winres make --arch amd64,arm64
go test .
rm -rf dist && mkdir dist
for arch in amd64 arm64; do
  GOOS=windows GOARCH=$arch go build -trimpath -ldflags="-H windowsgui -s -w" -o dist/$arch/otp.exe .
  (cd dist/$arch && zip -q ../otp4windows-$arch.exe.zip otp.exe)
done
ls -l dist/*.zip
