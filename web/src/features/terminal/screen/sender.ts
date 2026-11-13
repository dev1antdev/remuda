// The screen terminal's typing on its way: strokes queue while a request is out and go together in the next one, so
// keystrokes typed quickly are one request, in order, and one pane never has two requests in flight. A refusal drops
// what was queued behind it — nothing typed is sent again or later by itself.

import { takeBatch, type Batch, type Stroke } from "./strokes";

export type Sender = {
  push: (stroke: Stroke) => void;
  busy: () => boolean;
};

export function createSender(send: (batch: Batch) => Promise<void>, hooks: { sent: () => void; failed: (error: unknown) => void }): Sender {
  let queue: Stroke[] = [];
  let out = false;
  const pump = async () => {
    if (out) return;
    const next = takeBatch(queue);
    if (!next) return;
    queue = next.rest;
    out = true;
    try {
      await send(next.batch);
      hooks.sent();
    } catch (error) {
      queue = [];
      hooks.failed(error);
    } finally {
      out = false;
    }
    void pump();
  };
  return {
    push: (stroke) => {
      queue.push(stroke);
      void pump();
    },
    busy: () => out || queue.length > 0,
  };
}
