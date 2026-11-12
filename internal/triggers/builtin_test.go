package triggers

import "testing"

// The rows dev servers and test runners print, as a terminal shows them once herdr strips the colours.
func TestTheBuiltInWatchesKnowTheUsualWords(t *testing.T) {
	serves := map[string]int{
		"  ➜  Local:   http://localhost:5173/":                                                    5173, // Vite
		"   - Local:        http://localhost:3000":                                                3000, // Next.js
		"  Local:            http://localhost:3000":                                               3000, // Create React App
		" ┃ Local    http://localhost:4321/":                                                      4321, // Astro
		"<i> [webpack-dev-server] Loopback: http://localhost:8080/":                               8080,
		"Serving HTTP on 0.0.0.0 port 8000 (http://0.0.0.0:8000/) ...":                            8000, // python -m http.server
		"INFO:     Uvicorn running on http://127.0.0.1:8000 (Press CTRL+C to quit)":               8000,
		" * Running on http://127.0.0.1:5000":                                                     5000, // Flask
		"Starting development server at http://127.0.0.1:8000/":                                   8000, // Django
		"* Listening on http://127.0.0.1:3000":                                                    3000, // Puma
		"listening on :8080":                                                                      8080,
		"Server listening on port 4000":                                                           4000,
		"Server running at http://localhost:3001/":                                                3001,
		"ready - started server on 0.0.0.0:3000, url: http://localhost:3000":                      3000, // Next.js 12
		"Web Server is available at http://localhost:1313/ (bind address 127.0.0.1)":              1313, // Hugo
		"    Server address: http://127.0.0.1:4000/":                                              4000, // Jekyll
		"[Thu Nov 12 14:00:00 2026] PHP 8.3.0 Development Server (http://localhost:8080) started": 8080,
		"  ➜  Local:   http://[::1]:5174/":                                                        5174,
	}
	for line, port := range serves {
		if !Serves(line) {
			t.Errorf("not seen as a dev server: %q", line)
			continue
		}
		if got, _, ok := ServedAt(line); !ok || got != port {
			t.Errorf("port of %q = %d, want %d", line, got, port)
		}
	}
	for _, line := range []string{
		"I will start the dev server locally and check it",
		"The listener is on the local network",
		"  ➜  Network: use --host to expose",
		"listening on the socket",
		"server is running fine",
	} {
		if Serves(line) {
			t.Errorf("seen as a dev server: %q", line)
		}
	}
	if _, _, ok := ServedAt("Serving HTTP on 0.0.0.0 port 80 (http://0.0.0.0:80/)"); ok {
		t.Error("a port under 1024 is no preview")
	}

	failed := []string{
		"--- FAIL: TestReview (0.02s)",
		"FAIL    github.com/acme/api/internal/review    0.214s",
		"FAIL    github.com/acme/api/internal/review [build failed]",
		"Tests:       2 failed, 40 passed, 42 total",                           // jest
		"      Tests  2 failed | 40 passed (42)",                               // vitest
		" Test Files  1 failed | 12 passed (13)",                               // vitest
		"=================== 2 failed, 10 passed in 0.12s ===================", // pytest
		"FAILED tests/test_api.py::test_login - AssertionError",
		"test result: FAILED. 10 passed; 2 failed; 0 ignored; 0 measured",
		"  2 failing", // mocha
		"12 examples, 3 failures",
		"# fail 1", // node --test
	}
	for _, line := range failed {
		if !TestsFailed(line) {
			t.Errorf("not seen as failed tests: %q", line)
		}
	}
	for _, line := range []string{
		"ok      github.com/acme/api/internal/review    0.214s",
		"Tests:       42 passed, 42 total",
		"      Tests  42 passed (42)",
		"=================== 10 passed in 0.12s ===================",
		"test result: ok. 10 passed; 0 failed",
		"12 examples, 0 failures",
		"# fail 0",
		"The test FAILED because the fixture is missing", // an agent's prose
		"why did --- FAIL: happen",
	} {
		if TestsFailed(line) {
			t.Errorf("seen as failed tests: %q", line)
		}
	}
}
