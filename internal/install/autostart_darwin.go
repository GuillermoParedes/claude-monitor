package install

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/GuillermoParedes/claude-monitor/internal/paths"
)

const agentLabel = "com.github.guillermoparedes.claude-monitor"

func plistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", agentLabel+".plist")
}

func enableAutostart(bin string, o Options) error {
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key>
  <array><string>%s</string><string>run</string></array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><dict><key>SuccessfulExit</key><false/></dict>
  <key>ProcessType</key><string>Interactive</string>
  <key>StandardErrorPath</key><string>%s</string>
</dict>
</plist>
`, agentLabel, bin, paths.LogFile())
	if err := os.MkdirAll(filepath.Dir(plistPath()), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(plistPath(), []byte(plist), 0o644); err != nil {
		return err
	}
	o.say("✓ inicio     LaunchAgent %s", plistPath())
	if !o.NoStart {
		uid := fmt.Sprint(os.Getuid())
		_ = exec.Command("launchctl", "bootout", "gui/"+uid+"/"+agentLabel).Run()
		if out, err := exec.Command("launchctl", "bootstrap", "gui/"+uid, plistPath()).CombinedOutput(); err != nil {
			return fmt.Errorf("launchctl bootstrap: %v: %s", err, out)
		}
	}
	return nil
}

func disableAutostart(o Options) {
	uid := fmt.Sprint(os.Getuid())
	_ = exec.Command("launchctl", "bootout", "gui/"+uid+"/"+agentLabel).Run()
	_ = os.Remove(plistPath())
	o.say("✓ LaunchAgent retirado")
}

func stopRunning() {
	uid := fmt.Sprint(os.Getuid())
	_ = exec.Command("launchctl", "bootout", "gui/"+uid+"/"+agentLabel).Run()
	_ = exec.Command("pkill", "-f", "claude-monitor (run|tray)").Run()
}

func installWindowsTray(o Options) error { return nil }
func uninstallWindowsTray(o Options)     {}

// Headless solo aplica a Linux/WSL.
func Headless() bool { return false }
