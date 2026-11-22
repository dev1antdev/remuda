// Package wintest is what the tests run on a real Windows need: CI's windows job (.github/workflows/ci.yml) installs
// remuda-core.exe with its ConPTY, builds remuda.exe, and makes Git Bash the shell of the runner's own OpenSSH server,
// then runs the packages' windows_test.go files with REMUDA_WINDOWS_CORE=1. They reach remuda-core through
// `remuda-core.exe connect` over that ssh, as a cockpit on a Mac or Linux does, and run the remote scripts the way it
// does: every script through Windows' ssh on its stdin as sshx.Remote sends it.
package wintest
