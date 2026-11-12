package scenes

import (
	"errors"
	"fmt"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/dev1antdev/remuda/internal/herdr"
)

// MaxPanes is the most panes a captured layout lays out.
const MaxPanes = 8

// Layout is a tab's panes as a step lays them out again: herdr's tree without its pane ids, every folder inside the
// step's own kept relative to it, so the step's folder may move; herdr's environment variables are not kept.
type Layout struct {
	Root  Node `json:"root"`
	Agent int  `json:"agent"` // the pane the step's agent starts in, counted left to right and top to bottom
}

// Node is a pane or a split of the layout.
type Node struct {
	Type      string   `json:"type"`              // pane | split
	Dir       string   `json:"dir,omitempty"`     // a pane's folder: relative to the step's, "" for it, or absolute outside it
	Label     string   `json:"label,omitempty"`   // a pane's own name
	Command   []string `json:"command,omitempty"` // what herdr runs in the pane instead of a shell
	Direction string   `json:"direction,omitempty"`
	Ratio     float64  `json:"ratio,omitempty"`
	First     *Node    `json:"first,omitempty"`
	Second    *Node    `json:"second,omitempty"`
}

// panes are the layout's pane nodes in herdr's order.
func (n *Node) panes() []*Node {
	if n == nil {
		return nil
	}
	if n.Type != "split" {
		return []*Node{n}
	}
	return append(n.First.panes(), n.Second.panes()...)
}

// Capture is a tab's layout as a step keeps it: the folders made relative to dir, the agent's pane found by its id
// (the first pane when it is not in the tab).
func Capture(desc *herdr.LayoutDescription, dir, agentPane string) Layout {
	layout := Layout{Root: nodeOf(&desc.Root, path.Clean(dir))}
	for i, pane := range desc.Root.Panes() {
		if pane.PaneID == agentPane {
			layout.Agent = i
			break
		}
	}
	return layout
}

func nodeOf(n *herdr.LayoutNode, dir string) Node {
	if n.Type == "split" && n.First != nil && n.Second != nil {
		first, second := nodeOf(n.First, dir), nodeOf(n.Second, dir)
		return Node{Type: "split", Direction: n.Direction, Ratio: n.Ratio, First: &first, Second: &second}
	}
	return Node{Type: "pane", Dir: relativeTo(dir, n.CWD), Label: n.Label, Command: n.Command}
}

// relativeTo is a folder as seen from dir: "" for dir itself, the path below it, else the folder as it is.
func relativeTo(dir, folder string) string {
	if folder == "" {
		return ""
	}
	folder = path.Clean(folder)
	switch {
	case folder == dir:
		return ""
	case strings.HasPrefix(folder, dir+"/"):
		return strings.TrimPrefix(folder, dir+"/")
	case dir == "/":
		return strings.TrimPrefix(folder, "/")
	}
	return folder
}

// Tree is the layout as herdr's layout.apply takes it, every folder absolute again under dir.
func (l Layout) Tree(dir string) herdr.LayoutNode {
	return treeOf(&l.Root, path.Clean(dir))
}

func treeOf(n *Node, dir string) herdr.LayoutNode {
	if n.Type == "split" {
		first, second := treeOf(n.First, dir), treeOf(n.Second, dir)
		return herdr.LayoutNode{Type: "split", Direction: n.Direction, Ratio: n.Ratio, First: &first, Second: &second}
	}
	folder := dir
	if strings.HasPrefix(n.Dir, "/") {
		folder = n.Dir
	} else if n.Dir != "" {
		folder = path.Join(dir, n.Dir)
	}
	return herdr.LayoutNode{Type: "pane", CWD: folder, Label: n.Label, Command: n.Command}
}

// AgentPane is the pane of a laid-out tab the step's agent starts in: the one at the layout's index, else the first.
func (l Layout) AgentPane(laid *herdr.LayoutNode) string {
	panes := laid.Panes()
	if len(panes) == 0 {
		return ""
	}
	if l.Agent >= 0 && l.Agent < len(panes) {
		return panes[l.Agent].PaneID
	}
	return panes[0].PaneID
}

// checkLayout refuses a tree herdr could not lay out, or one remuda would not: past MaxPanes, a split with a direction
// or a share herdr does not take, a folder that climbs out of the step's, a label too long, a command with a NUL, the
// agent's pane missing or running a command of its own.
func checkLayout(l Layout) error {
	panes := l.Root.panes()
	if len(panes) == 0 || len(panes) > MaxPanes {
		return fmt.Errorf("a layout of one to %d panes", MaxPanes)
	}
	if err := checkNode(&l.Root, 0); err != nil {
		return err
	}
	if l.Agent < 0 || l.Agent >= len(panes) {
		return errors.New("the layout's agent pane is not one of its panes")
	}
	if len(panes[l.Agent].Command) > 0 {
		return errors.New("the layout's agent pane runs a command of its own")
	}
	return nil
}

func checkNode(n *Node, depth int) error {
	if depth > MaxPanes {
		return errors.New("the layout nests too deep")
	}
	switch n.Type {
	case "split":
		if n.Direction != "right" && n.Direction != "down" {
			return fmt.Errorf("a split goes right or down, not %q", n.Direction)
		}
		if n.Ratio < 0.05 || n.Ratio > 0.95 {
			return errors.New("a split gives each side 5 to 95 %")
		}
		if n.First == nil || n.Second == nil {
			return errors.New("a split has two sides")
		}
		if err := checkNode(n.First, depth+1); err != nil {
			return err
		}
		return checkNode(n.Second, depth+1)
	case "pane":
		if n.First != nil || n.Second != nil || n.Direction != "" {
			return errors.New("a pane holds no split")
		}
		if n.Dir != "" && !strings.HasPrefix(n.Dir, "/") && (path.Clean(n.Dir) != n.Dir || n.Dir == ".." || strings.HasPrefix(n.Dir, "../")) {
			return fmt.Errorf("a pane's folder %q climbs out of the step's", n.Dir)
		}
		if utf8.RuneCountInString(n.Label) > MaxTab || !plain(n.Label) {
			return fmt.Errorf("a pane's label of at most %d characters, no control characters", MaxTab)
		}
		if len(n.Command) > 64 {
			return errors.New("a pane's command of at most 64 words")
		}
		for _, word := range n.Command {
			if strings.ContainsRune(word, 0) {
				return errors.New("a pane's command holds a NUL")
			}
		}
		return nil
	}
	return fmt.Errorf("a layout holds panes and splits, not %q", n.Type)
}

// Capture is a workspace's active tab as a step lays it out again: its layout, the workspace's folder and the agent
// running in the tab, if any.
func (s *Service) Capture(machineID, workspaceID string) (Layout, string, string, error) {
	mc, err := s.Machine(machineID)
	if err != nil {
		return Layout{}, "", "", err
	}
	client, err := mc.Herdr()
	if err != nil {
		return Layout{}, "", "", err
	}
	snap := mc.Snapshot()
	if snap == nil {
		return Layout{}, "", "", errors.New("no snapshot of the machine yet")
	}
	ws := snap.Workspace(workspaceID)
	if ws == nil {
		return Layout{}, "", "", errors.New("that workspace is not open")
	}
	desc, err := client.LayoutExport(ws.ActiveTabID)
	if err != nil {
		return Layout{}, "", "", err
	}
	dir := snap.WorkspaceDir(ws)
	agentPane, agent := "", ""
	for i := range snap.Panes {
		if pane := &snap.Panes[i]; pane.TabID == ws.ActiveTabID && pane.Agent != "" {
			agentPane, agent = pane.PaneID, pane.Agent
			break
		}
	}
	layout := Capture(desc, dir, agentPane)
	if err := checkLayout(layout); err != nil {
		return Layout{}, "", "", fmt.Errorf("that tab cannot be kept as a layout: %w", err)
	}
	return layout, dir, agent, nil
}
