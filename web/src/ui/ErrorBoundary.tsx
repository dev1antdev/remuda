// ErrorBoundary keeps one broken piece from blanking the page: React unmounts the whole tree on a render error
// unless a boundary catches it, so a turn the parser cannot draw shows a note and the rest stands. The one class
// component here, because React gives boundaries no hook.

import { Component, type ReactNode } from "react";
import { errMsg } from "@/lib/text";

export class ErrorBoundary extends Component<{ children: ReactNode; fallback: (message: string) => ReactNode }, { message: string | null }> {
  state = { message: null as string | null };

  static getDerivedStateFromError(error: unknown) {
    return { message: errMsg(error) };
  }

  componentDidCatch(error: unknown) {
    console.error(error);
  }

  render() {
    return this.state.message === null ? this.props.children : this.props.fallback(this.state.message);
  }
}
