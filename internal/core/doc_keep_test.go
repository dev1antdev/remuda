package core_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/core"
	"github.com/dev1antdev/remuda/internal/core/coretest"
	"github.com/dev1antdev/remuda/internal/herdr"
)

func TestAnEditKeepsWhatANewerRemudaWroteInTheDocument(t *testing.T) {
	client := coretest.Server(t).Client(t)
	ctx := within(t, 30*time.Second)
	newer := `{"workspaces":[{"id":"ws-1","label":"api","folder":"/tmp","tabs":[{"id":"tab-1","label":"","pinned":true}],"color":"teal","worktree":{"checkout_path":"/p/api","repo_key":"/p/api/.git","repo_name":"api","repo_root":"/p/api","is_linked_worktree":false}}],"order":"manual"}`
	if _, err := client.DocSet(ctx, core.DocName, json.RawMessage(newer), 0); err != nil {
		t.Fatal(err)
	}
	adapter := adapterOf(t, client, core.AdapterHooks{})
	eventually(t, adapter, func(*herdr.Snapshot) bool { return true })
	if err := adapter.WorkspaceRename("ws-1", "renamed"); err != nil {
		t.Fatal(err)
	}
	doc, err := client.DocGet(ctx, core.DocName)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"order":"manual"`, `"color":"teal"`, `"pinned":true`, `"label":"renamed"`, `"repo_root":"/p/api"`} {
		if !strings.Contains(string(doc.Value), field) {
			t.Errorf("the edit lost %s: %s", field, doc.Value)
		}
	}
}
