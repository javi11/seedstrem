import { useEffect, useState } from "react";
import { Outlet, useNavigate } from "react-router-dom";
import { api, SESSION_EXPIRED_EVENT } from "../api";
import { applyTheme, getStoredTheme, ThemeName, toggleTheme } from "../lib/theme";
import { Sidebar } from "./Sidebar";
import { OfflineBanner } from "./OfflineBanner";
import { ConfirmDialog } from "./ConfirmDialog";
import { Skeleton } from "./Skeleton";
import { NavigationGuardProvider } from "./NavigationGuard";

// The authenticated application shell: left sidebar on desktop, a top bar +
// slide-in drawer on mobile, plus app-wide offline and session-expired handling.
export function AppShell() {
  const navigate = useNavigate();
  const [checked, setChecked] = useState(false);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [expired, setExpired] = useState(false);
  const [theme, setTheme] = useState<ThemeName>(getStoredTheme);

  useEffect(() => {
    applyTheme(theme);
  }, [theme]);

  useEffect(() => {
    api
      .sessionInfo()
      .then(() => setChecked(true))
      .catch(() => navigate("/login"));
  }, [navigate]);

  useEffect(() => {
    const onExpired = () => setExpired(true);
    window.addEventListener(SESSION_EXPIRED_EVENT, onExpired);
    return () => window.removeEventListener(SESSION_EXPIRED_EVENT, onExpired);
  }, []);

  const onToggleTheme = () => setTheme((t) => toggleTheme(t));

  const onLogout = () => {
    api
      .logout()
      .catch(() => {})
      .finally(() => navigate("/login"));
  };

  if (!checked) {
    return (
      <div className="min-h-screen bg-base-200 p-4">
        <div className="mx-auto max-w-5xl space-y-4 pt-8">
          <Skeleton className="h-8 w-48" />
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-64 w-full" />
        </div>
      </div>
    );
  }

  return (
    <NavigationGuardProvider>
    <div className="min-h-screen bg-base-200 md:flex">
      {/* Desktop rail */}
      <aside className="sticky top-0 hidden h-screen shrink-0 border-r border-base-content/10 md:block">
        <Sidebar theme={theme} onToggleTheme={onToggleTheme} onLogout={onLogout} />
      </aside>

      {/* Mobile top bar */}
      <div className="flex items-center gap-2 border-b border-base-content/10 bg-base-100 px-3 py-2 md:hidden">
        <button
          className="btn btn-ghost btn-sm"
          aria-label="Open menu"
          onClick={() => setDrawerOpen(true)}
        >
          ☰
        </button>
        <span className="flex items-center gap-2 font-bold tracking-brand">
          <span className="grid h-7 w-7 place-items-center rounded-field bg-gradient-to-br from-primary to-accent text-sm">
            🌱
          </span>
          seedstrem
        </span>
      </div>

      {/* Mobile drawer */}
      {drawerOpen && (
        <div className="fixed inset-0 z-40 md:hidden">
          <button
            className="absolute inset-0 bg-black/50"
            aria-label="Close menu"
            onClick={() => setDrawerOpen(false)}
          />
          <div className="absolute left-0 top-0 h-full shadow-xl">
            <Sidebar
              theme={theme}
              onToggleTheme={onToggleTheme}
              onLogout={onLogout}
              onNavigate={() => setDrawerOpen(false)}
            />
          </div>
        </div>
      )}

      <div className="flex min-w-0 flex-1 flex-col">
        <OfflineBanner />
        <main className="mx-auto w-full max-w-5xl flex-1 p-4 md:p-6">
          <Outlet />
        </main>
      </div>

      <ConfirmDialog
        open={expired}
        title="Session expired"
        confirmLabel="Go to login"
        cancelLabel="Dismiss"
        onCancel={() => setExpired(false)}
        onConfirm={() => {
          setExpired(false);
          navigate("/login");
        }}
      >
        You&rsquo;ve been signed out. Log in again to continue.
      </ConfirmDialog>
    </div>
    </NavigationGuardProvider>
  );
}
