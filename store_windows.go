package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Accounts are stored in %APPDATA%\otp4windows\accounts.dat as JSON,
// encrypted with DPAPI so only the current Windows user can read them.
// Preferences are not secret and go to settings.json next to it.

func dataPath(name string) (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "otp4windows", name), nil
}

func writeFile(name string, data []byte) error {
	path, err := dataPath(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readFile(name string) ([]byte, error) {
	path, err := dataPath(name)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return data, err
}

func loadAccounts() ([]Account, error) {
	data, err := readFile("accounts.dat")
	if err != nil {
		return nil, err
	}
	if data == nil {
		return migrateSecret()
	}
	plain, err := unprotect(data)
	if err != nil {
		return nil, err
	}
	var accounts []Account
	if err := json.Unmarshal(plain, &accounts); err != nil {
		return nil, err
	}
	return accounts, nil
}

func saveAccounts(accounts []Account) error {
	plain, err := json.Marshal(accounts)
	if err != nil {
		return err
	}
	data, err := protect(plain)
	if err != nil {
		return err
	}
	return writeFile("accounts.dat", data)
}

// migrateSecret turns the single secret.dat of earlier versions into an
// account list.
func migrateSecret() ([]Account, error) {
	data, err := readFile("secret.dat")
	if err != nil || data == nil {
		return nil, err
	}
	plain, err := unprotect(data)
	if err != nil {
		return nil, err
	}
	secret := normalizeSecret(string(plain))
	if !validSecret(secret) {
		return nil, nil
	}
	accounts := []Account{{
		ID: newID(), Secret: secret,
		Algorithm: "SHA1", Digits: 6, Period: 30,
	}}
	if err := saveAccounts(accounts); err != nil {
		return nil, err
	}
	if path, err := dataPath("secret.dat"); err == nil {
		os.Remove(path)
	}
	return accounts, nil
}

func loadPreferences() Preferences {
	p := defaultPreferences()
	if data, err := readFile("settings.json"); err == nil && data != nil {
		json.Unmarshal(data, &p)
	}
	return p
}

func savePreferences(p Preferences) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return writeFile("settings.json", data)
}

func protect(plain []byte) ([]byte, error) {
	if len(plain) == 0 {
		return nil, errors.New("nothing to encrypt")
	}
	in := windows.DataBlob{Size: uint32(len(plain)), Data: &plain[0]}
	var out windows.DataBlob
	if err := windows.CryptProtectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}

func unprotect(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("empty data file")
	}
	in := windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}
