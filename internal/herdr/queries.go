package herdr

import (
	"bytes"
	"regexp"
	"strconv"
)

// An attach stream carries terminal queries — herdr's own when it attaches (DECRQM for the pixel mouse mode, the
// primary device attributes) and an app's when it starts. Every viewer's xterm would answer each of them, and the answers
// come back late and one per viewer, typed into the pane as text. So the server answers them once, at once, as xterm.js
// would, and the viewers never see them.

// queryRe matches the queries answered here: DA1 (CSI c, CSI 0 c), DA2 (CSI > c, CSI > 0 c) and DECRQM (CSI ? Pn $ p).
var queryRe = regexp.MustCompile(`\x1b\[(?:(0?c)|(>0?c)|\?(\d+)\$p)`)

// queryPrefixRe matches a chunk's tail that may be the start of a query cut by the chunk boundary.
var queryPrefixRe = regexp.MustCompile(`\x1b(?:\[(?:0|>0?|\?\d*\$?)?)?$`)

// knownModes are the DEC private modes xterm.js reports as known but reset (2); any other is reported unknown (0).
var knownModes = map[int]bool{1: true, 6: true, 7: true, 12: true, 25: true, 45: true, 47: true, 66: true, 1000: true, 1002: true, 1003: true, 1004: true, 1005: true, 1006: true, 1015: true, 1016: true, 1047: true, 1048: true, 1049: true, 2004: true}

// queryScanner answers the queries in a stream chunk by chunk, holding back a tail that may be a query cut in two.
type queryScanner struct {
	carry []byte
}

// scan takes the next chunk of the stream and answers with what the viewers get and what the terminal is told.
func (q *queryScanner) scan(chunk []byte) (forward, replies []byte) {
	data := append(q.carry, chunk...)
	q.carry = nil
	if tail := queryPrefixRe.Find(data); tail != nil {
		q.carry = append([]byte(nil), tail...)
		data = data[:len(data)-len(tail)]
	}
	last := 0
	for _, m := range queryRe.FindAllSubmatchIndex(data, -1) {
		forward = append(forward, data[last:m[0]]...)
		last = m[1]
		replies = append(replies, reply(data, m)...)
	}
	forward = append(forward, data[last:]...)
	return forward, replies
}

// flush hands back whatever tail is held, for a stream that ends.
func (q *queryScanner) flush() []byte {
	tail := q.carry
	q.carry = nil
	return tail
}

// reply is xterm.js's own answer to one matched query.
func reply(data []byte, m []int) []byte {
	switch {
	case m[2] >= 0:
		return []byte("\x1b[?1;2c")
	case m[4] >= 0:
		return []byte("\x1b[>0;276;0c")
	default:
		mode, _ := strconv.Atoi(string(data[m[6]:m[7]]))
		state := "0"
		if knownModes[mode] {
			state = "2"
		}
		return []byte("\x1b[?" + strconv.Itoa(mode) + ";" + state + "$y")
	}
}

// hasQuery says whether a chunk carries any query at all, so a scanner is only asked when it matters.
func hasQuery(chunk []byte) bool {
	return bytes.Contains(chunk, []byte{0x1b, '['})
}
