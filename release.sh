#!/bin/bash
#
# go install github.com/tc-hib/go-winres@latest
# sudo apt install jq wixl msitools gh default-jre
# jsign: install jsign_<version>_all.deb from https://github.com/ebourg/jsign/releases
#
# Code signing uses the Certum certificate in SimplySign, the key never leaves
# the Certum cloud. Install SimplySign Desktop for Linux, start it and log in
# with a token from the SimplySign mobile app before running this script.
# Signing uses jsign, not osslsigncode: the SimplySign PKCS#11 library has
# OpenSSL 1.0 built in and crashes when loaded next to the system OpenSSL 3.
# Settings (environment variables):
#
#   CERTUM_MODULE  PKCS#11 library of SimplySign Desktop (searched in /opt)
#   CERTUM_CERT    signing certificate followed by the intermediate CA, PEM
#                  (default ~/.config/certum/codesign.pem)
#   CERTUM_ALIAS   alias of the key, when SimplySign has more than one
#   CERTUM_PIN     PKCS#11 PIN, if SimplySign asks for one
#
# Releases a new version of otp4win:
#
#   ./release.sh          # 0.3.3 -> 0.3.4 (build, the default)
#   ./release.sh minor    # 0.3.4 -> 0.4.0
#   ./release.sh major    # 0.4.0 -> 1.0.0
#
# The version is file_version in winres/winres.json, as
# "major.minor.build.revision". Windows Installer only compares the first
# three numbers, so every release raises at least the build. The revision
# goes up on every release as well.
#
# The script builds dist/otp4win-<version>-amd64.msi, the portable
# dist/otp4win-<version>-amd64.exe (the same exe the MSI installs), the
# portable dist/otp4win-<version>-arm64.exe (wixl cannot build an arm64 MSI;
# Windows 11 on ARM runs the amd64 one emulated) and dist/SHA256SUMS, commits
# and pushes the version bump and creates the GitHub release v<version> with
# those four files.
set -e
cd "$(dirname "$0")"

part=${1:-build}
case $part in major | minor | build) ;; *)
	echo "usage: $0 [major|minor|build]" >&2
	exit 1
	;;
esac
if [ -n "$(git status --porcelain)" ]; then
	echo "working tree not clean, commit first" >&2
	exit 1
fi

# Code signing.
module=${CERTUM_MODULE:-$(find /opt -iname 'SimplySignPKCS*.so' 2>/dev/null | head -1)}
cert=${CERTUM_CERT:-$HOME/.config/certum/codesign.pem}
if [ ! -f "$module" ] || [ ! -f "$cert" ] || ! command -v jsign >/dev/null; then
	echo "cannot sign: install jsign, set CERTUM_MODULE and CERTUM_CERT, see the top of $0" >&2
	exit 1
fi
sign() {
	# jsign takes a SunPKCS11 configuration, not the library itself.
	local cfg=dist/simplysign.cfg
	printf 'name = SimplySign\nlibrary = %s\n' "$module" >$cfg
	jsign --storetype PKCS11 --keystore $cfg --storepass "${CERTUM_PIN:-}" \
		${CERTUM_ALIAS:+--alias "$CERTUM_ALIAS"} --certfile "$cert" \
		--alg SHA-256 --name otp4win --url https://github.com/mevdschee/otp4win \
		--tsaurl http://time.certum.pl --tsmode RFC3161 "$1"
	rm $cfg
}

# Bump the version.
json=winres/winres.json
ver=$(jq -r '.RT_VERSION["#1"]["0000"].fixed.file_version' $json)
IFS=. read -r major minor build revision <<<"$ver"
case $part in
major) major=$((major + 1)) minor=0 build=0 ;;
minor) minor=$((minor + 1)) build=0 ;;
build) build=$((build + 1)) ;;
esac
revision=$((revision + 1))
version=$major.$minor.$build
jq --arg v "$version" --arg fv "$version.$revision" '
  .RT_VERSION["#1"]["0000"].fixed.file_version = $fv |
  .RT_VERSION["#1"]["0000"].fixed.product_version = $fv |
  .RT_VERSION["#1"]["0000"].info["0409"].FileVersion = $v |
  .RT_VERSION["#1"]["0000"].info["0409"].ProductVersion = $v
' $json >$json.tmp && mv $json.tmp $json
echo "Releasing otp4win $version (revision $revision)"

# Build the exe: icon, manifest and version info (rsrc_windows_amd64.syso,
# not committed) and the version for the update check.
~/go/bin/go-winres make --arch amd64,arm64
go test .
rm -rf dist && mkdir dist
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-H windowsgui -s -w -X main.version=$version" -o dist/otp4win.exe .
arm=dist/otp4win-$version-arm64.exe
GOOS=windows GOARCH=arm64 go build -trimpath -ldflags="-H windowsgui -s -w -X main.version=$version" -o $arm .
# Sign the exes before the MSI is built, so the MSI installs the signed exe.
sign dist/otp4win.exe
sign $arm

# Build the MSI.
msi=dist/otp4win-$version-amd64.msi
go run ./tools/mkico dist/otp4win.ico winres/icon.png winres/icon48.png winres/icon32.png winres/icon16.png
wixl -a x64 -D Version="$version" -D FileVersion="$version.$revision" \
	-D ExePath=dist/otp4win.exe -D IconPath=dist/otp4win.ico \
	-o $msi installer/otp4win.wxs

# wixl removes the old version right after InstallValidate, outside the
# installation transaction: if the new install then fails, nothing is left.
# Move RemoveExistingProducts after InstallInitialize, so a failed upgrade
# rolls back to the old version (WiX: Schedule="afterInstallInitialize").
msiinfo export $msi InstallExecuteSequence >dist/seq.idt
sed -i 's/^RemoveExistingProducts\t\t1401\r\?$/RemoveExistingProducts\t\t1501\r/' dist/seq.idt
grep -q $'^RemoveExistingProducts\t\t1501\r$' dist/seq.idt
msibuild $msi -i dist/seq.idt
rm dist/seq.idt
# The exe must be versioned in the File table, see installer/otp4win.wxs.
msiinfo export $msi File | grep -q $'\t'"$version.$revision"$'\t'
# Sign the MSI last, any change to it after this breaks the signature.
sign $msi

# The portable amd64 exe and the checksums.
exe=dist/otp4win-$version-amd64.exe
cp dist/otp4win.exe $exe
(cd dist && sha256sum "$(basename $msi)" "$(basename $exe)" "$(basename $arm)" >SHA256SUMS)
ls -l $msi $exe $arm
cat dist/SHA256SUMS

# Commit, push and publish.
git commit -q -m "Release v$version" $json
git push
gh release create v$version --title v$version $msi $exe $arm dist/SHA256SUMS
