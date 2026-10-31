# otp4win

A TOTP (two-factor authentication code) client for Windows, written in Go with
[Windigo](https://github.com/rodrigocfd/windigo). It has the features of
[open-otp](https://github.com/avan1235/open-otp), but uses plain Win32 controls
and GDI drawing (no OpenGL), so it is small, starts fast and works well over RDP.

![otp4win in light and dark mode](screenshot.png)

## Features

- Each code is shown on a card with the issuer initial, "Issuer: account", the
  code and a countdown bar that empties and turns red in the last 5 seconds.
- Click a card to copy its code to the clipboard.
- Add codes by hand with issuer, account name, secret, algorithm
  (SHA1/SHA256/SHA512), digits (6/8) and period (15/30/60 seconds).
- Paste an `otpauth://` link to add a code at once, either in the main window or
  in any field of the add dialog. Google Authenticator exports
  (`otpauth-migration://`) add all their accounts.
- Search on issuer and account name.
- Reorder with drag and drop, or sort by issuer or account name with optional
  group headers, empty values first and reverse order.
- Delete from the right-click menu, with optional confirmation.
- Uses the Windows colors and follows the light/dark app mode and high contrast
  themes automatically. Light or dark can also be forced in the settings.
- Scales with the Windows display scaling (DPI).
- Checks for a newer release on startup and installs it on request.

HOTP (counter based) codes are not supported.

## Keyboard

| Key                    | Action                                       |
| ---------------------- | -------------------------------------------- |
| Ctrl+V                 | Add the `otpauth://` link from the clipboard |
| Ctrl+N                 | Add a code                                   |
| Ctrl+F                 | Search                                       |
| Esc                    | Clear the search                             |
| Up, Down, Home, End    | Select a code                                |
| Enter, Space or Ctrl+C | Copy the selected code                       |
| Ctrl+Up, Ctrl+Down     | Move the selected code (when not sorted)     |
| Delete                 | Delete the selected code                     |

## Install

Download `otp4win-<version>-amd64.msi` from the
[releases](https://github.com/mevdschee/otp4win/releases) and run it. It
installs `otp4win.exe` in Program Files and adds an "otp4win" shortcut to the
Start menu. Newer versions replace older ones. The installer is not signed, so
Windows SmartScreen may ask you to confirm ("More info", "Run anyway").

It runs on 64-bit Windows 10 and 11.

## Data

Codes are saved in `%APPDATA%\otp4win\accounts.dat`, encrypted with DPAPI so
only the current Windows user can read them. Settings are in `settings.json`
next to it. Your codes never leave your computer.

## Updates

On startup otp4win asks GitHub for the latest release. When a newer version
is available it offers to download the MSI and install it; otp4win closes while
the installer runs. This is the only network access and can be turned off in
Settings ("Check for updates on startup").

## Build (on Linux)

Install the tools once:

    sudo apt install jq wixl msitools
    go install github.com/tc-hib/go-winres@latest

The icon, manifest (visual styles, DPI awareness) and version info are compiled
from `winres/` into `rsrc_windows_amd64.syso`, which is not committed. Generate
it before building (`go generate` needs `~/go/bin` in your `PATH`):

    go generate
    GOOS=windows GOARCH=amd64 go build -ldflags="-H windowsgui -s -w" -o otp4win.exe .

## Test

    go test .
    wine otp4win.exe

## Release

The version is set in `winres/winres.json` (`file_version`, as
`major.minor.patch.build`). Change the first three numbers by hand for a new
version; the build number is bumped automatically.

    ./package.sh   # bump build number, build dist/otp4win-<version>-amd64.msi
    ./release.sh   # create GitHub release v<major.minor.patch> with the MSI

`release.sh` runs `package.sh` when the MSI is missing and takes an optional
release title as argument. The installer is defined in `installer/otp4win.wxs`.
