// Package undo takes back one agent turn in a git folder, at the person's word: a checkpoint of the folder when an
// agent pane starts working and another when it comes to rest, read on the machine through a throwaway index and
// object folder (nothing written into the repository), the turn's delta kept on the cockpit, and an undo that checks
// every file still holds what the turn left, then puts the uncommitted changes back directly and reverts the turn's
// commits with git revert, a new commit each. It is the one place remuda writes into a project's working tree.
package undo
