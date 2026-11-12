package secrets

import (
	"strings"
	"unicode/utf8"
)

// excerptContext is how many bytes of the line an excerpt keeps on either side of its secret.
const excerptContext = 40

// bullets stand in for a secret's body.
const bullets = "••••••••"

// mask is a secret as an excerpt shows it: the kind's prefix, bullets, and the last four bytes of a long one unless
// noTail says its end is the secret's too (a private key's body).
func mask(secret string, keep int, noTail bool) string {
	if keep >= len(secret) {
		return secret // a private key's header, which gives the key away but holds none of it
	}
	tail := ""
	if len(secret)-keep >= 32 && !noTail {
		tail = secret[len(secret)-4:]
	}
	return secret[:keep] + bullets + tail
}

// excerpt is the line around hits[focus], every secret in it masked: one the window would cut through is taken in
// whole, so no part of it shows unmasked.
func excerpt(line string, hits []hit, focus int) string {
	target := hits[focus]
	from, to := max(0, target.start-excerptContext), min(len(line), target.end+excerptContext)
	for _, other := range hits {
		if other.start < from && other.end > from {
			from = other.start
		}
		if other.start < to && other.end > to {
			to = other.end
		}
	}
	for from > 0 && !utf8.RuneStart(line[from]) {
		from--
	}
	for to < len(line) && !utf8.RuneStart(line[to]) {
		to++
	}
	var middle strings.Builder
	at := from
	for _, other := range hits {
		if other.end <= from || other.start >= to {
			continue
		}
		middle.WriteString(line[at:other.start])
		middle.WriteString(mask(line[other.start:other.end], other.keep, other.noTail))
		at = other.end
	}
	middle.WriteString(line[at:to])
	shown := strings.TrimSpace(strings.Map(blankControl, strings.ToValidUTF8(middle.String(), "")))
	if from > 0 {
		shown = "…" + shown
	}
	if to < len(line) {
		shown += "…"
	}
	return shown
}

// blankControl turns a tab or any other control character into a space.
func blankControl(r rune) rune {
	if r < ' ' || r == 0x7f {
		return ' '
	}
	return r
}
