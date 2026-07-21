import { Component, ErrorInfo, ReactNode } from "react";

interface Props {
  children: ReactNode;
}

interface State {
  error: Error | null;
}

// Catches render-time errors anywhere below it so a single bad render shows a
// friendly recovery card instead of a blank white screen.
export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo): void {
    // Surface to the console for debugging; no telemetry in a self-hosted app.
    console.error("Unhandled UI error:", error, info.componentStack);
  }

  render(): ReactNode {
    if (!this.state.error) return this.props.children;

    return (
      <div className="flex min-h-screen items-center justify-center bg-base-200 p-4">
        <div className="surface max-w-md p-8 text-center">
          <div className="text-5xl">🌱</div>
          <h1 className="mt-4 text-xl font-bold tracking-brand">Something went wrong</h1>
          <p className="mt-2 text-sm opacity-70">
            The page hit an unexpected error. Reloading usually clears it.
          </p>
          <pre className="mt-4 max-h-32 overflow-auto rounded-lg bg-base-200 p-3 text-left font-mono text-xs opacity-70">
            {this.state.error.message}
          </pre>
          <button
            className="btn btn-primary mt-6"
            onClick={() => window.location.reload()}
          >
            Reload page
          </button>
        </div>
      </div>
    );
  }
}
