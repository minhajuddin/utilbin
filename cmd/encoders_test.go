package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/minhajuddin/utilbin/internal/registry"
)

// runCmd resolves a registered command by name (primary, alias, or shim) and
// runs it via registry.Dispatch — same path the BusyBox-style symlink
// dispatcher takes — capturing whatever the command writes.
func runCmd(t *testing.T, name string, args ...string) string {
	t.Helper()
	// Make sure cobra is wired up so that commands have parents linking back
	// to Root. AttachAll is idempotent for our purposes (cobra's AddCommand
	// is not, so guard with a sync.Once-style sentinel).
	ensureAttached()

	c := registry.Find(name)
	if c == nil {
		t.Fatalf("no registered command named %q", name)
	}
	var buf bytes.Buffer
	root := c.Root()
	root.SetOut(&buf)
	root.SetErr(&buf)
	if err := registry.Dispatch(c, args); err != nil {
		t.Fatalf("%s %v: %v\noutput: %s", name, args, err, buf.String())
	}
	return buf.String()
}

var attached bool

func ensureAttached() {
	if attached {
		return
	}
	AttachAll()
	attached = true
}

func TestURLEncodeDecodeRoundTrip(t *testing.T) {
	encoded := strings.TrimSpace(runCmd(t, "urlencode", "hello world =1&b=2"))
	if encoded != "hello+world+%3D1%26b%3D2" {
		t.Fatalf("unexpected encoding: %q", encoded)
	}
	decoded := strings.TrimSpace(runCmd(t, "urldecode", encoded))
	if decoded != "hello world =1&b=2" {
		t.Fatalf("round-trip mismatch: %q", decoded)
	}
}

func TestBase64RoundTrip(t *testing.T) {
	enc := strings.TrimSpace(runCmd(t, "base64encode", "foo bar"))
	if enc != "Zm9vIGJhcg==" {
		t.Fatalf("unexpected base64: %q", enc)
	}
	// base64decode writes raw bytes; no trailing newline so don't trim.
	if got := runCmd(t, "base64decode", enc); got != "foo bar" {
		t.Fatalf("round-trip mismatch: %q", got)
	}
}

func TestRandHexLength(t *testing.T) {
	out := strings.TrimSpace(runCmd(t, "randhex", "16"))
	if len(out) != 32 {
		t.Fatalf("randhex 16 produced %d chars, want 32", len(out))
	}
}

func TestUUIDFormat(t *testing.T) {
	out := strings.TrimSpace(runCmd(t, "uuid"))
	if len(out) != 36 || strings.Count(out, "-") != 4 {
		t.Fatalf("uuid output looks wrong: %q", out)
	}
}

func TestRegistryDispatch(t *testing.T) {
	for _, name := range []string{"urlencode", "urldecode", "base64encode", "base64decode", "rand", "uuid", "randhex", "randbase64"} {
		if registry.Find(name) == nil {
			t.Errorf("expected %q to be registered", name)
		}
	}
}
