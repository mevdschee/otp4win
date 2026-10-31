#!/bin/bash
#
# go install github.com/tc-hib/go-winres@latest
# sudo apt install jq wixl msitools
#
# Builds dist/otp4win-<version>-amd64.msi. The version is read from
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
echo "Building otp4win $version (build $build)"

# icon, manifest and version info (rsrc_windows_amd64.syso, not committed)
~/go/bin/go-winres make --arch amd64
go test .
rm -rf dist && mkdir dist
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-H windowsgui -s -w -X main.version=$version" -o dist/otp4win.exe .

go run ./tools/mkico dist/otp4win.ico winres/icon.png winres/icon48.png winres/icon32.png winres/icon16.png
wixl -a x64 -D Version="$version" -D FileVersion="$version.$build" \
  -D ExePath=dist/otp4win.exe -D IconPath=dist/otp4win.ico \
  -o dist/otp4win-$version-amd64.msi installer/otp4win.wxs

# wixl removes the old version right after InstallValidate, outside the
# installation transaction: if the new install then fails, nothing is left.
# Move RemoveExistingProducts after InstallInitialize, so a failed upgrade
# rolls back to the old version (WiX: Schedule="afterInstallInitialize").
msiinfo export dist/otp4win-$version-amd64.msi InstallExecuteSequence > dist/seq.idt
sed -i 's/^RemoveExistingProducts\t\t1401\r\?$/RemoveExistingProducts\t\t1501\r/' dist/seq.idt
grep -q $'^RemoveExistingProducts\t\t1501\r$' dist/seq.idt
msibuild dist/otp4win-$version-amd64.msi -i dist/seq.idt
rm dist/seq.idt
# the exe must be versioned in the File table, see installer/otp4win.wxs
msiinfo export dist/otp4win-$version-amd64.msi File | grep -q $'\t'"$version.$build"$'\t'
ls -l dist/*.msi
