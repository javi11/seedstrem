import {
  createContext,
  ReactNode,
  useCallback,
  useContext,
  useRef,
  useState,
} from "react";
import { useNavigate } from "react-router-dom";
import { ConfirmDialog } from "./ConfirmDialog";

// Lets a page (Settings) register a predicate that, when true, intercepts
// in-app navigation to confirm discarding unsaved work. Works with the plain
// <Routes> setup (no data router / useBlocker required): navigation flows
// through requestNavigate() instead of raw <NavLink> clicks.

type Guard = () => boolean;

interface NavigationGuardApi {
  setGuard: (guard: Guard | null) => void;
  requestNavigate: (to: string) => void;
}

const Ctx = createContext<NavigationGuardApi | null>(null);

export function NavigationGuardProvider({ children }: { children: ReactNode }) {
  const navigate = useNavigate();
  const guardRef = useRef<Guard | null>(null);
  const [pending, setPending] = useState<string | null>(null);

  const setGuard = useCallback((guard: Guard | null) => {
    guardRef.current = guard;
  }, []);

  const requestNavigate = useCallback(
    (to: string) => {
      if (guardRef.current?.()) {
        setPending(to);
      } else {
        navigate(to);
      }
    },
    [navigate],
  );

  return (
    <Ctx.Provider value={{ setGuard, requestNavigate }}>
      {children}
      <ConfirmDialog
        open={pending !== null}
        title="Discard unsaved changes?"
        confirmLabel="Discard & leave"
        cancelLabel="Stay"
        danger
        onCancel={() => setPending(null)}
        onConfirm={() => {
          const to = pending;
          setPending(null);
          guardRef.current = null; // avoid re-prompting during the navigation
          if (to) navigate(to);
        }}
      >
        You have unsaved settings. Leaving this page will discard them.
      </ConfirmDialog>
    </Ctx.Provider>
  );
}

export function useNavigationGuard(): NavigationGuardApi {
  const ctx = useContext(Ctx);
  if (!ctx) throw new Error("useNavigationGuard must be used within provider");
  return ctx;
}
