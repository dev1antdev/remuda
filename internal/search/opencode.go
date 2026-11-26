// opencode's conversations are a database, not files: its matches travel as one line each, after the files' lines, as
// opencode<US>folder<US>session<US>message<US>role<US>time<US>snippet.

package search

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/transcript"
)

// ScanOpencode writes opencode's matching messages since a moment, newest first, up to limit: nothing when the machine
// has no opencode database (or no sqlite3 to read it), which is no failed search.
func ScanOpencode(host shell.Host, q, dir string, since time.Time, limit int, w io.Writer) error {
	hits, err := transcript.NewReader(host).OpencodeSearch(q, dir, since, limit)
	if err != nil {
		return nil
	}
	for _, hit := range hits {
		snippet := strings.ReplaceAll(hit.Snippet, script.US, " ")
		fmt.Fprintf(w, "opencode%s%s%s%s%s%s%s%s%s%s%s%s\n", script.US, hit.Dir, script.US, hit.Session, script.US, hit.UUID, script.US, hit.Role, script.US, hit.At.UTC().Format(time.RFC3339), script.US, snippet)
	}
	return nil
}
