//go:generate go run ./tools/genicon
//go:generate go-winres make --arch amd64

package main

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/dim13/otpauth/migration"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/hotp"
)

// Account is one stored TOTP secret. The fields mirror what open-otp lets
// you enter: issuer, account name, secret and the advanced settings.
type Account struct {
	ID          string `json:"id"`
	Issuer      string `json:"issuer,omitempty"`
	AccountName string `json:"accountName,omitempty"`
	Secret      string `json:"secret"`
	Algorithm   string `json:"algorithm"` // SHA1, SHA256 or SHA512
	Digits      int    `json:"digits"`    // 6 or 8
	Period      int    `json:"period,omitempty"`
}

var (
	Algorithms = []string{"SHA1", "SHA256", "SHA512"}
	DigitsList = []int{6, 8}
	Periods    = []int{15, 30, 60}
)

func newID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// normalizeSecret cleans up a pasted base32 secret: it drops whitespace,
// dashes and padding, and upper-cases the rest.
func normalizeSecret(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case ' ', '\t', '\r', '\n', '-', '=':
			continue
		default:
			b.WriteRune(r)
		}
	}
	return strings.ToUpper(b.String())
}

// validSecret reports whether s is a usable normalized base32 secret.
func validSecret(s string) bool {
	if s == "" {
		return false
	}
	_, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(s)
	return err == nil
}

func (a *Account) algorithm() otp.Algorithm {
	switch a.Algorithm {
	case "SHA256":
		return otp.AlgorithmSHA256
	case "SHA512":
		return otp.AlgorithmSHA512
	default:
		return otp.AlgorithmSHA1
	}
}

func (a *Account) digits() int {
	if a.Digits == 8 {
		return 8
	}
	return 6
}

func (a *Account) period() int64 {
	if a.Period > 0 {
		return int64(a.Period)
	}
	return 30
}

// Code returns the TOTP code at t (RFC 6238: HOTP over the time step).
func (a *Account) Code(t time.Time) (string, error) {
	counter := uint64(t.Unix() / a.period())
	return hotp.GenerateCodeCustom(a.Secret, counter, hotp.ValidateOpts{
		Digits:    otp.Digits(a.digits()),
		Algorithm: a.algorithm(),
	})
}

// TimeLeft returns the fraction (0..1] of the TOTP period that is left at t.
func (a *Account) TimeLeft(t time.Time) float64 {
	p := a.period() * 1000
	ms := t.UnixMilli() % p
	return float64(p-ms) / float64(p)
}

// NamePresentation is the "Issuer: account" line shown above the code, or ""
// when both are empty. Like open-otp, a duplicated issuer prefix is dropped.
func (a *Account) NamePresentation() string {
	issuer := strings.TrimSpace(a.Issuer)
	name := strings.TrimSpace(a.AccountName)
	if issuer != "" {
		name = strings.TrimPrefix(name, issuer+": ")
		name = strings.TrimPrefix(name, issuer+":")
	}
	switch {
	case issuer != "" && name != "":
		return issuer + ": " + name
	case issuer != "":
		return issuer
	default:
		return name
	}
}

// splitCode formats "123456" as "123 456", like open-otp.
func splitCode(code string) string {
	h := len(code) / 2
	return code[:h] + " " + code[h:]
}

// Theme values for Preferences.Theme.
const (
	ThemeLight  = "light"
	ThemeDark   = "dark"
	ThemeSystem = "system"
)

// SortBy values for Preferences.SortBy.
const (
	SortNone        = "none"
	SortIssuer      = "issuer"
	SortAccountName = "accountName"
)

// Preferences holds the settings screen state. Defaults match open-otp.
type Preferences struct {
	Theme         string `json:"theme"`
	ConfirmDelete bool   `json:"confirmDelete"`
	SortBy        string `json:"sortBy"`
	NullsFirst    bool   `json:"nullsFirst"`
	Reversed      bool   `json:"reversed"`
	ShowHeaders   bool   `json:"showHeaders"`
	CheckUpdates  bool   `json:"checkUpdates"`
}

func defaultPreferences() Preferences {
	return Preferences{
		Theme:         ThemeSystem,
		ConfirmDelete: true,
		SortBy:        SortNone,
		ShowHeaders:   true,
		CheckUpdates:  true,
	}
}

// Row is one line of the code list: either a group header or an account
// (Index into the stored account slice).
type Row struct {
	Header   string
	IsHeader bool
	Index    int
}

func sortKey(a *Account, sortBy string) string {
	switch sortBy {
	case SortIssuer:
		return strings.TrimSpace(a.Issuer)
	case SortAccountName:
		return strings.TrimSpace(a.AccountName)
	}
	return ""
}

// presentRows applies the sort, grouping and search rules to accounts.
// A non-empty query filters on issuer and account name (case-insensitive)
// and, like open-otp's search results, shows no group headers.
func presentRows(accounts []Account, p Preferences, query string) []Row {
	idx := make([]int, 0, len(accounts))
	for i := range accounts {
		idx = append(idx, i)
	}
	sorting := p.SortBy == SortIssuer || p.SortBy == SortAccountName
	if sorting {
		slices.SortStableFunc(idx, func(x, y int) int {
			kx := strings.ToLower(sortKey(&accounts[x], p.SortBy))
			ky := strings.ToLower(sortKey(&accounts[y], p.SortBy))
			var c int
			switch {
			case kx == ky:
				c = 0
			case kx == "":
				c = 1 // empty values last
				if p.NullsFirst {
					c = -1
				}
			case ky == "":
				c = -1
				if p.NullsFirst {
					c = 1
				}
			default:
				c = strings.Compare(kx, ky)
			}
			if p.Reversed {
				c = -c
			}
			return c
		})
	}

	q := strings.ToLower(strings.TrimSpace(query))
	headers := sorting && p.ShowHeaders && q == ""
	rows := make([]Row, 0, len(idx))
	var group string
	for n, i := range idx {
		a := &accounts[i]
		if q != "" && !strings.Contains(strings.ToLower(a.Issuer), q) &&
			!strings.Contains(strings.ToLower(a.AccountName), q) {
			continue
		}
		if headers {
			// Groups are formed on the exact key, as open-otp does.
			key := sortKey(a, p.SortBy)
			if n == 0 || key != group {
				group = key
				title := key
				if title == "" {
					title = "Empty issuer"
					if p.SortBy == SortAccountName {
						title = "Empty account name"
					}
				}
				rows = append(rows, Row{Header: title, IsHeader: true})
			}
		}
		rows = append(rows, Row{Index: i})
	}
	return rows
}

// moveAccount moves the account at from to position to, keeping the rest in
// order.
func moveAccount(accounts []Account, from, to int) []Account {
	if from == to || from < 0 || to < 0 || from >= len(accounts) || to >= len(accounts) {
		return accounts
	}
	a := accounts[from]
	accounts = slices.Delete(accounts, from, from+1)
	return slices.Insert(accounts, to, a)
}

// parseOtpauth parses an otpauth:// URI as produced by QR codes, e.g.
// otpauth://totp/Issuer:account?secret=...&issuer=Issuer&digits=6.
// Some generators encode labels twice (%2520, %2B), so values are decoded
// until they no longer change.
func parseOtpauth(s string) (Account, error) {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil {
		return Account{}, err
	}
	if !strings.EqualFold(u.Scheme, "otpauth") {
		return Account{}, errors.New("not an otpauth URI")
	}
	if !strings.EqualFold(u.Host, "totp") {
		return Account{}, fmt.Errorf("only TOTP codes are supported, not %q", u.Host)
	}
	a := Account{Algorithm: "SHA1", Digits: 6, Period: 30}

	label := decodeFully(strings.TrimPrefix(u.EscapedPath(), "/"))
	issuer, name, found := strings.Cut(label, ":")
	if !found {
		issuer, name = "", label
	}
	a.Issuer = strings.TrimSpace(issuer)
	a.AccountName = strings.TrimSpace(name)

	q := u.Query()
	if a.Issuer == "" { // like fyne-authenticator, the label prefix wins
		a.Issuer = strings.TrimSpace(decodeFully(q.Get("issuer")))
	}
	a.Secret = normalizeSecret(q.Get("secret"))
	if v := strings.ToUpper(q.Get("algorithm")); slices.Contains(Algorithms, v) {
		a.Algorithm = v
	}
	if v, err := strconv.Atoi(q.Get("digits")); err == nil && slices.Contains(DigitsList, v) {
		a.Digits = v
	}
	if v, err := strconv.Atoi(q.Get("period")); err == nil && slices.Contains(Periods, v) {
		a.Period = v
	}
	return a, nil
}

// parseOtpURIs parses an otpauth:// URI or a Google Authenticator export
// (otpauth-migration://offline?data=...), which holds several accounts.
// HOTP accounts in an export are skipped.
func parseOtpURIs(s string) ([]Account, error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(strings.ToLower(s), "otpauth-migration:") {
		a, err := parseOtpauth(s)
		if err != nil {
			return nil, err
		}
		return []Account{a}, nil
	}
	payload, err := migration.UnmarshalURL(s)
	if err != nil {
		return nil, err
	}
	var accounts []Account
	for _, op := range payload.OtpParameters {
		if op.Type == migration.Payload_OtpParameters_OTP_TYPE_HOTP {
			continue
		}
		a, err := parseOtpauth(op.URL().String())
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	if len(accounts) == 0 {
		return nil, errors.New("no TOTP accounts in export")
	}
	return accounts, nil
}

// decodeFully undoes repeated URL encoding, treating "+" as a space.
func decodeFully(s string) string {
	for range 3 {
		d, err := url.QueryUnescape(s)
		if err != nil || d == s {
			break
		}
		s = d
	}
	return s
}
