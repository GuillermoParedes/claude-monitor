package tray

import (
	"strings"
	"testing"
)

func TestSafeTextNeutralizesPowerShellAndXML(t *testing.T) {
	for _, in := range []string{
		`$(Start-Process calc): espera permiso`,
		"proj`$x",
		"a\"@\n$(evil)",
		"x]]><text>pwn</text>",
	} {
		out := safeText(in)
		for _, bad := range []string{"$", "`", `"`, "]]>", "\n"} {
			if strings.Contains(out, bad) {
				t.Errorf("safeText(%q) = %q still contains %q", in, out, bad)
			}
		}
	}
	if got := safeText("casper: espera permiso"); got != "casper: espera permiso" {
		t.Errorf("normal text altered: %q", got)
	}
}
