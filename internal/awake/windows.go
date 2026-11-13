package awake

import (
	"encoding/base64"
	"unicode/utf16"
)

// windowsHoldCode is the PowerShell that keeps a Windows machine from sleeping for as long as it runs: the system
// stays required (ES_CONTINUOUS | ES_SYSTEM_REQUIRED), the display may sleep, and the hold goes with the process.
const windowsHoldCode = `Add-Type -Name Hold -Namespace RemudaAwake -MemberDefinition '[DllImport("kernel32.dll")] public static extern uint SetThreadExecutionState(uint flags);'
[RemudaAwake.Hold]::SetThreadExecutionState([uint32]2147483649) | Out-Null
while ($true) { Start-Sleep -Seconds 3600 }`

// windowsHold is the code as PowerShell's -EncodedCommand takes it, base64 of UTF-16LE: no quoting survives two shells.
var windowsHold = encodeCommand(windowsHoldCode)

func encodeCommand(code string) string {
	units := utf16.Encode([]rune(code))
	raw := make([]byte, 0, 2*len(units))
	for _, u := range units {
		raw = append(raw, byte(u), byte(u>>8))
	}
	return base64.StdEncoding.EncodeToString(raw)
}
