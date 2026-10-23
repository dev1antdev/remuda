// The page counts as visible while this document is, or any other document drawn from it: the pop-out window
// stays on screen when the tab behind it is hidden, and the polls that feed it must go on.

const others = new Set<Document>();

// alsoVisible adds a document of ours to the test; the return takes it out again.
export function alsoVisible(doc: Document): () => void {
  others.add(doc);
  return () => {
    others.delete(doc);
  };
}

export const pageVisible = () => document.visibilityState === "visible" || [...others].some((doc) => doc.visibilityState === "visible");
