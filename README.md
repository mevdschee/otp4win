# otp4windows

A TOTP client for Windows, written in Go with [Windigo](https://github.com/rodrigocfd/windigo).
It has the features of [open-otp](https://github.com/avan1235/open-otp), but
uses plain Win32 controls and GDI drawing (no OpenGL), so it works well over RDP.

## Features

- A list of code cards showing the issuer initial, "Issuer: account", the code
  and a countdown bar that empties as the code turns from green to red.
- Click a card (or press Enter) to copy its code.
- Search on issuer and account name (Ctrl+F, Esc clears).
- Add codes by hand (Ctrl+N) with issuer, account name, secret, algorithm
  (SHA1/SHA256/SHA512), digits (6/8) and period (15/30/60 s).
- Paste an `otpauth://` link (Ctrl+V in the main window, or in any field of the
  add dialog) to fill in everything at once. Google Authenticator exports
  (`otpauth-migration://`) add all their TOTP accounts. HOTP (counter based)
  codes are not supported.
- Delete with the Delete key or the right-click menu, with optional confirmation.
- Reorder with drag and drop or Ctrl+Up/Down, or sort by issuer or account name
  with optional group headers, empty values first and reverse order.
- Light, dark or system theme.

Codes are saved in `%APPDATA%\otp4windows\accounts.dat`, encrypted with DPAPI
for the current Windows user. Settings are in `settings.json` next to it. The
`secret.dat` of earlier versions is migrated on first start.

## Build (on Linux)

    GOOS=windows GOARCH=amd64 go build -ldflags="-H windowsgui -s -w" -o otp.exe .

The icon, manifest and version info live in `rsrc_windows_*.syso`, generated
from `winres/`. To regenerate them:

    go install github.com/tc-hib/go-winres@latest
    go generate

## Release

The version is set in `winres/winres.json` (`file_version`, as
`major.minor.patch.build`). Change the first three numbers by hand for a new
version; the build number is bumped automatically.

    ./package.sh   # bump build number, build dist/otp4windows-{amd64,arm64}.exe.zip
    ./release.sh   # create GitHub release v<major.minor.patch> with the zips

`release.sh` runs `package.sh` when the zips are missing and takes an optional
release title as argument. It needs `jq`, `zip`, `go-winres` and the GitHub CLI.

## Test

    go test .
    wine otp.exe
