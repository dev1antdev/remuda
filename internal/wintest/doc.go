// Package wintest is what the tests run on a real Windows need: CI's windows job (.github/workflows/ci.yml) installs
// herdr for Windows, starts its server, builds remuda.exe, and makes Git Bash the shell of the runner's own OpenSSH
// server, then runs the packages' windows_test.go files with REMUDA_WINDOWS_HERDR=1. They drive herdr and run the
// remote scripts the way a cockpit on a Mac or Linux does over ssh: every script through Windows' ssh as
// sshx.RemoteCommand words it, herdr's RPC through `remuda.exe bridge`.
package wintest
