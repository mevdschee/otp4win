#!/bin/bash
#
# go install github.com/tc-hib/go-winres@latest
# sudo apt install wine
#
# Cross compiles otp4win.exe for Windows and runs it with Wine:
#
#   ./run.sh
#
# Wine keeps its own %APPDATA%, so the codes and settings are stored in
# ~/.wine/drive_c/users/$USER/AppData/Roaming/otp4win.
set -e
cd "$(dirname "$0")"

# Check the dependencies.
missing=
command -v go >/dev/null || missing="$missing go"
command -v wine >/dev/null || missing="$missing wine"
winres=$(command -v go-winres || echo ~/go/bin/go-winres)
[ -x "$winres" ] || missing="$missing go-winres"
if [ -n "$missing" ]; then
	echo "missing:$missing" >&2
	echo "install with: sudo apt install golang wine" >&2
	echo "              go install github.com/tc-hib/go-winres@latest" >&2
	exit 1
fi

# Build the exe with icon, manifest and version info.
"$winres" make --arch amd64
GOOS=windows GOARCH=amd64 go build -ldflags="-H windowsgui" -o otp4win.exe .

# Run it.
exec wine otp4win.exe "$@"
