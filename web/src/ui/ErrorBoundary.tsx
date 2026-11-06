// ErrorBoundary keeps one broken piece from blanking the page: React unmounts the whole tree on a render error
// unless a boundary catches it, so a turn the parser cannot draw shows a note and the rest stands. The one class
// component here, because React gives boundaries no hook. A new resetKey (the turn as the next poll brings it) tries
// the children again, so a piece that broke on one reading does not stay a note once it can be drawn.

import { Component, type ReactNode } from "react";
import { errMsg } from "@/lib/text";

type Props = { children: ReactNode; fallback: (message: string) => ReactNode; resetKey?: unknown };
type State = { message: string | null; resetKey: unknown };

export class ErrorBoundary extends Component<Props, State> {
  state: State = { message: null, resetKey: this.props.resetKey };

  static getDerivedStateFromProps(props: Props, state: State): Partial<State> | null {
    return Object.is(props.resetKey, state.resetKey) ? null : { message: null, resetKey: props.resetKey };
  }

  static getDerivedStateFromError(error: unknown): Partial<State> {
    return { message: errMsg(error) };
  }

  componentDidCatch(error: unknown) {
    console.error(error);
  }

  render() {
    return this.state.message === null ? this.props.children : this.props.fallback(this.state.message);
  }
}
