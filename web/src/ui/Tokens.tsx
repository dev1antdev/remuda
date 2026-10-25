// The highlighter's tokens as React nodes: a span per coloured token, plain text as it is. The chat's code fences
// and the Changes lens's diff rows draw through this, so the classes (.hl-<kind>) are named in one place.

import type { ReactNode } from "react";
import type { Token } from "@/lib/highlight";

export function tokenNodes(tokens: Token[], keyPrefix = ""): ReactNode[] {
  return tokens.map((token, i) => (token.kind === "plain" ? token.text : <span key={`${keyPrefix}${i}`} className={`hl-${token.kind}`}>{token.text}</span>));
}
