// A long piece of work in the page cut into slices of a few milliseconds, so the page answers between them however big
// each step is: a GIF frame of a wide pane is a hundred times a small one's. Pure.

export const SLICE_MS = 30;

// Slicer says when the slice under way has run its time
export class Slicer {
  private started: number;

  constructor(
    private readonly now: () => number,
    private readonly budget = SLICE_MS,
  ) {
    this.started = now();
  }

  // due says the work should hand the page its turn now
  get due(): boolean {
    return this.now() - this.started >= this.budget;
  }

  // restart begins the next slice, once the page had its turn
  restart() {
    this.started = this.now();
  }
}
