package main

import (
	"github.com/dim13/otpauth/migration"
	"google.golang.org/protobuf/proto"
	"reflect"
	"testing"
	"time"
)

func TestNormalizeSecret(t *testing.T) {
	got := normalizeSecret(" jbsw y3dp-ehpk 3pxp==\r\n")
	if got != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("got %q", got)
	}
}

func TestValidSecret(t *testing.T) {
	for s, want := range map[string]bool{
		"JBSWY3DPEHPK3PXP": true,
		"":                 false,
		"JBSWY3DP1":        false, // 1 is not in the base32 alphabet
	} {
		if got := validSecret(s); got != want {
			t.Errorf("validSecret(%q) = %v, want %v", s, got, want)
		}
	}
}

// RFC 6238 test vectors. The seeds are "12345678901234567890" repeated to
// the HMAC key length for each algorithm.
func TestTOTPCode(t *testing.T) {
	const (
		sha1   = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
		sha256 = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQGEZA"
		sha512 = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQGEZDGNA"
	)
	for _, tc := range []struct {
		secret, alg string
		unix        int64
		want        string
	}{
		{sha1, "SHA1", 59, "94287082"},
		{sha256, "SHA256", 59, "46119246"},
		{sha512, "SHA512", 59, "90693936"},
		{sha1, "SHA1", 1111111109, "07081804"},
		{sha256, "SHA256", 1234567890, "91819424"},
		{sha512, "SHA512", 20000000000, "47863826"},
	} {
		a := Account{Secret: tc.secret, Algorithm: tc.alg, Digits: 8, Period: 30}
		got, err := a.Code(time.Unix(tc.unix, 0))
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("%s at %d: got %s, want %s", tc.alg, tc.unix, got, tc.want)
		}
		a.Digits = 6
		got, _ = a.Code(time.Unix(tc.unix, 0))
		if got != tc.want[2:] {
			t.Errorf("%s at %d (6 digits): got %s, want %s", tc.alg, tc.unix, got, tc.want[2:])
		}
	}
}

func TestTimeLeft(t *testing.T) {
	a := Account{Period: 30}
	if got := a.TimeLeft(time.UnixMilli(60_000)); got != 1 {
		t.Errorf("at start of period: got %v", got)
	}
	if got := a.TimeLeft(time.UnixMilli(75_000)); got != 0.5 {
		t.Errorf("halfway: got %v", got)
	}
}

func TestNamePresentation(t *testing.T) {
	for _, tc := range []struct{ issuer, name, want string }{
		{"GitHub", "maurits", "GitHub: maurits"},
		{"GitHub", "GitHub: maurits", "GitHub: maurits"},
		{"GitHub", "", "GitHub"},
		{"", "maurits", "maurits"},
		{" ", " ", ""},
	} {
		a := Account{Issuer: tc.issuer, AccountName: tc.name}
		if got := a.NamePresentation(); got != tc.want {
			t.Errorf("(%q, %q): got %q, want %q", tc.issuer, tc.name, got, tc.want)
		}
	}
}

func TestSplitCode(t *testing.T) {
	if got := splitCode("123456"); got != "123 456" {
		t.Errorf("got %q", got)
	}
	if got := splitCode("12345678"); got != "1234 5678" {
		t.Errorf("got %q", got)
	}
}

func TestPresentRows(t *testing.T) {
	accounts := []Account{
		{Issuer: "b", AccountName: "x"},
		{Issuer: "", AccountName: "y"},
		{Issuer: "a", AccountName: "z"},
		{Issuer: "b", AccountName: "w"},
	}
	p := defaultPreferences()

	if got, want := presentRows(accounts, p, ""), []Row{{Index: 0}, {Index: 1}, {Index: 2}, {Index: 3}}; !reflect.DeepEqual(got, want) {
		t.Errorf("unsorted: got %v", got)
	}

	p.SortBy = SortIssuer
	want := []Row{
		{Header: "a", IsHeader: true}, {Index: 2},
		{Header: "b", IsHeader: true}, {Index: 0}, {Index: 3},
		{Header: "Empty issuer", IsHeader: true}, {Index: 1},
	}
	if got := presentRows(accounts, p, ""); !reflect.DeepEqual(got, want) {
		t.Errorf("by issuer: got %v", got)
	}

	p.NullsFirst = true
	p.ShowHeaders = false
	if got, want := presentRows(accounts, p, ""), []Row{{Index: 1}, {Index: 2}, {Index: 0}, {Index: 3}}; !reflect.DeepEqual(got, want) {
		t.Errorf("nulls first: got %v", got)
	}

	p.NullsFirst = false
	p.Reversed = true
	if got, want := presentRows(accounts, p, ""), []Row{{Index: 1}, {Index: 0}, {Index: 3}, {Index: 2}}; !reflect.DeepEqual(got, want) {
		t.Errorf("reversed: got %v", got)
	}

	p = defaultPreferences()
	p.SortBy = SortAccountName
	if got, want := presentRows(accounts, p, "B"), []Row{{Index: 3}, {Index: 0}}; !reflect.DeepEqual(got, want) {
		t.Errorf("search: got %v", got)
	}
}

func TestMoveAccount(t *testing.T) {
	ids := func(as []Account) (s string) {
		for _, a := range as {
			s += a.ID
		}
		return
	}
	as := []Account{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}}
	as = moveAccount(as, 0, 2)
	if got := ids(as); got != "bcad" {
		t.Errorf("got %s", got)
	}
	as = moveAccount(as, 3, 0)
	if got := ids(as); got != "dbca" {
		t.Errorf("got %s", got)
	}
}

func TestParseOtpauth(t *testing.T) {
	a, err := parseOtpauth("otpauth://totp/Huisartsenprakijk%2520Kastelenbuurt:info@huisartsenkastelenbuurt.nl?issuer=Huisartsenprakijk%2BKastelenbuurt&secret=jbsw%20y3dp-ehpk3pxp")
	if err != nil {
		t.Fatal(err)
	}
	want := Account{Issuer: "Huisartsenprakijk Kastelenbuurt",
		AccountName: "info@huisartsenkastelenbuurt.nl", Secret: "JBSWY3DPEHPK3PXP",
		Algorithm: "SHA1", Digits: 6, Period: 30}
	if a != want {
		t.Errorf("got %+v", a)
	}

	a, err = parseOtpauth("otpauth://totp/acme?secret=GEZDGNBV&algorithm=sha256&digits=8&period=60")
	if err != nil {
		t.Fatal(err)
	}
	want = Account{AccountName: "acme", Secret: "GEZDGNBV",
		Algorithm: "SHA256", Digits: 8, Period: 60}
	if a != want {
		t.Errorf("got %+v", a)
	}

	if _, err := parseOtpauth("otpauth://hotp/acme?secret=GEZDGNBV&counter=7"); err == nil {
		t.Error("expected error for HOTP URI")
	}

	if _, err := parseOtpauth("https://example.com"); err == nil {
		t.Error("expected error for non-otpauth URI")
	}
}

func TestParseOtpURIsMigration(t *testing.T) {
	// Google Authenticator export with one TOTP account "Example:alice"
	// (issuer "Example", secret JBSWY3DPEHPK3PXP) and one HOTP account,
	// which is skipped.
	op := &migration.Payload_OtpParameters{
		Secret: []byte("Hello!\xde\xad\xbe\xef"), Name: "Example:alice", Issuer: "Example",
		Algorithm: migration.Payload_OtpParameters_ALGORITHM_SHA1,
		Digits:    migration.Payload_OtpParameters_DIGIT_COUNT_SIX,
		Type:      migration.Payload_OtpParameters_OTP_TYPE_TOTP,
	}
	hotp := &migration.Payload_OtpParameters{
		Secret: []byte("12345678901234567890"), Name: "counter",
		Type: migration.Payload_OtpParameters_OTP_TYPE_HOTP,
	}
	data, err := proto.Marshal(&migration.Payload{OtpParameters: []*migration.Payload_OtpParameters{op, hotp}})
	if err != nil {
		t.Fatal(err)
	}
	accounts, err := parseOtpURIs(migration.URL(data).String())
	if err != nil {
		t.Fatal(err)
	}
	want := Account{Issuer: "Example", AccountName: "alice",
		Secret: "JBSWY3DPEHPK3PXP", Algorithm: "SHA1", Digits: 6, Period: 30}
	if len(accounts) != 1 || accounts[0] != want {
		t.Errorf("got %+v", accounts)
	}
}
