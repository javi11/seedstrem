import { ThemeName } from "../lib/theme";
import { NavItem } from "./NavItem";

interface SidebarProps {
  theme: ThemeName;
  onToggleTheme: () => void;
  onLogout: () => void;
  onNavigate?: () => void;
}

// The persistent left navigation: brand, primary nav, and a footer with the
// theme toggle and logout. Rendered both in the desktop rail and inside the
// mobile drawer.
export function Sidebar({ theme, onToggleTheme, onLogout, onNavigate }: SidebarProps) {
  const dark = theme === "seedstrem-dark";
  return (
    <div className="flex h-full w-64 flex-col gap-1 bg-base-100 p-3">
      <div className="mb-4 flex items-center gap-2.5 px-2 py-2">
        <span className="grid h-9 w-9 place-items-center rounded-field bg-gradient-to-br from-primary to-accent text-lg shadow-md">
          🌱
        </span>
        <span className="text-lg font-bold tracking-brand">seedstrem</span>
      </div>

      <nav className="flex flex-col gap-1">
        <NavItem to="/" icon="▦" label="Dashboard" end onNavigate={onNavigate} />
        <NavItem to="/torrents" icon="↯" label="Torrents" onNavigate={onNavigate} />
        <NavItem to="/settings" icon="⚙" label="Settings" onNavigate={onNavigate} />
      </nav>

      <div className="mt-auto flex flex-col gap-1 border-t border-base-content/10 pt-2">
        <button
          className="flex items-center gap-3 rounded-field px-3 py-2.5 text-sm font-medium text-base-content/60 transition-colors hover:bg-base-300/60 hover:text-base-content"
          onClick={onToggleTheme}
        >
          <span className="w-5 text-center text-base" aria-hidden>
            {dark ? "☾" : "☀"}
          </span>
          {dark ? "Dark theme" : "Light theme"}
        </button>
        <button
          className="flex items-center gap-3 rounded-field px-3 py-2.5 text-sm font-medium text-base-content/60 transition-colors hover:bg-error/10 hover:text-error"
          onClick={onLogout}
        >
          <span className="w-5 text-center text-base" aria-hidden>
            ⏻
          </span>
          Log out
        </button>
      </div>
    </div>
  );
}
