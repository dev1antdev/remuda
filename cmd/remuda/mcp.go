package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/dev1antdev/remuda/internal/mcp"
	"github.com/dev1antdev/remuda/internal/setup"
	"github.com/dev1antdev/remuda/internal/tasksapi"
)

// mcpCmd is `remuda mcp`: the task list served over stdio to the coding agent that started it, or
// `remuda mcp install|status|uninstall [-host H]`, which registers the server with each agent through the agent's own command.
func mcpCmd(args []string) {
	action := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		action, args = args[0], args[1:]
	}
	switch action {
	case "":
		serveMCP()
	case "install", "status", "uninstall":
		fs := flag.NewFlagSet("mcp "+action, flag.ExitOnError)
		host := fs.String("host", "", "a remote machine over ssh instead of this one")
		fs.Parse(args)
		if err := setup.MCP(*host, action, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "mcp:", err)
			os.Exit(1)
		}
	default:
		fmt.Fprint(os.Stderr, `usage: remuda mcp                          serve the task list to an agent over stdio (what the agents run)
       remuda mcp install [-host H]       register it with every coding agent on the machine
       remuda mcp status|uninstall [-host H]
`)
		os.Exit(2)
	}
}

// serveMCP answers one agent until its input ends; without a door every tool says why, so the agent can tell the person.
func serveMCP() {
	tools := mcp.Tools{Source: os.Getenv("HERDR_PANE_ID")}
	if door, err := tasksapi.FindDoor(); err != nil {
		tools.Trouble = err
	} else {
		tools.Client = tasksapi.New(door)
	}
	if err := mcp.New(&tools, version).Serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "mcp:", err)
		os.Exit(1)
	}
}
