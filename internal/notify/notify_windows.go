package notify

import (
	"context"
	"os/exec"
	"strings"
)

// send raises a tray balloon through WinForms. It needs no extra PowerShell
// module, unlike the toast APIs, which matters for a tool people install
// themselves.
func send(ctx context.Context, title, body string) error {
	script := strings.Join([]string{
		`[reflection.assembly]::LoadWithPartialName('System.Windows.Forms') | Out-Null`,
		`$n = New-Object System.Windows.Forms.NotifyIcon`,
		`$n.Icon = [System.Drawing.SystemIcons]::Information`,
		`$n.BalloonTipTitle = '` + escapePowerShell(title) + `'`,
		`$n.BalloonTipText = '` + escapePowerShell(body) + `'`,
		`$n.Visible = $true`,
		`$n.ShowBalloonTip(10000)`,
		`Start-Sleep -Seconds 10`,
		`$n.Dispose()`,
	}, "; ")
	return exec.CommandContext(
		ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", script,
	).Run()
}
