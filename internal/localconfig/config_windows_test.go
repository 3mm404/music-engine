//go:build windows

package localconfig

import (
	"bytes"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validConfig() Config {
	return Config{Server: "https://example.test", Driver: "Test ASIO", SampleRate: 48000, Buffer: 256, Mode: "stereo"}
}
func TestCredentialPersistence(t *testing.T) {
	dir := t.TempDir()
	if err := Prepare(dir); err != nil {
		t.Fatal(err)
	}
	cfg := validConfig()
	const secret = "test-secret-not-for-production"
	if err := Save(dir, cfg, secret); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(secret)) {
		t.Fatal("plaintext credential persisted")
	}
	got, token, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if token != secret || got.Server != cfg.Server || got.Driver != cfg.Driver {
		t.Fatal("round trip failed")
	}
	cfg.Buffer = 512
	if err := Save(dir, cfg, "replacement"); err != nil {
		t.Fatal(err)
	}
	got, token, err = Load(dir)
	if err != nil || token != "replacement" || got.Buffer != 512 {
		t.Fatalf("replacement failed: %v", err)
	}
	if err := Save(dir, cfg, ""); err == nil {
		t.Fatal("accepted empty credential")
	}
	_, token, err = Load(dir)
	if err != nil || token != "replacement" {
		t.Fatal("failed save damaged previous settings")
	}
	got.Credential[len(got.Credential)/2] ^= 0xff
	if _, err := unprotect(got.Credential); err == nil {
		t.Fatal("accepted corrupted DPAPI data")
	}
}
func TestExclusiveLock(t *testing.T) {
	dir := t.TempDir()
	lock, err := Lock(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Lock(dir)
	if err == nil {
		second.Close()
		t.Fatal("second instance acquired lock")
	}
	lock.Close()
	lock, err = Lock(dir)
	if err != nil {
		t.Fatal(err)
	}
	lock.Close()
}
func TestValidation(t *testing.T) {
	for _, change := range []func(*Config){
		func(c *Config) { c.Server = "http://example.test" }, func(c *Config) { c.Server = "https://token@example.test" },
		func(c *Config) { c.Server = "https://example.test?token=secret" }, func(c *Config) { c.Driver = " " },
		func(c *Config) { c.Buffer = 0 }, func(c *Config) { c.SampleRate = 0 }, func(c *Config) { c.Mode = "bad" }, func(c *Config) { c.Channels = 65 },
	} {
		c := validConfig()
		change(&c)
		if c.Validate() == nil {
			t.Fatalf("accepted invalid settings: %+v", c)
		}
	}
}
func TestRestrictedDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := Prepare(dir); err != nil {
		t.Fatal(err)
	}
	sd, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	expected, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Replace(sd.String(), "D:PAI", "D:P", 1) != expected.String() {
		t.Fatalf("unexpected directory permissions: %s", sd.String())
	}
}

func TestBackendSwitchPersistsAndPreservesASIO(t *testing.T) {
	dir := t.TempDir()
	c := validConfig()
	if c.BackendName() != "asio" {
		t.Fatal("legacy default changed")
	}
	if err := Save(dir, c, "test-token"); err != nil {
		t.Fatal(err)
	}
	c, token, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	c.Backend = "oto"
	if err := Save(dir, c, token); err != nil {
		t.Fatal(err)
	}
	c, token, err = Load(dir)
	if err != nil || c.BackendName() != "oto" || c.Driver != "Test ASIO" {
		t.Fatalf("switch failed: %v", err)
	}
	c.Backend = "asio"
	if err := Save(dir, c, token); err != nil {
		t.Fatal(err)
	}
	c, _, err = Load(dir)
	if err != nil || c.Buffer != 256 {
		t.Fatal("lost ASIO settings")
	}
	c.Backend = "invalid"
	if c.Validate() == nil {
		t.Fatal("unknown backend accepted")
	}
}
