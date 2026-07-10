import { useEffect, useState } from "react";
import { NavLink, Outlet, useNavigate } from "react-router-dom";
import { api } from "./api";

export function Layout() {
  const navigate = useNavigate();
  const [checked, setChecked] = useState(false);

  useEffect(() => {
    api
      .sessionInfo()
      .then(() => setChecked(true))
      .catch(() => navigate("/login"));
  }, [navigate]);

  if (!checked) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <span className="loading loading-spinner loading-lg" />
      </div>
    );
  }

  const linkClass = ({ isActive }: { isActive: boolean }) =>
    isActive ? "btn btn-ghost btn-sm btn-active" : "btn btn-ghost btn-sm";

  return (
    <div className="min-h-screen bg-base-200">
      <div className="navbar bg-base-100 shadow-sm">
        <div className="flex-1">
          <span className="px-4 text-xl font-bold">
            🌱 seedstrem
          </span>
          <nav className="flex gap-1">
            <NavLink to="/" className={linkClass} end>
              Dashboard
            </NavLink>
            <NavLink to="/torrents" className={linkClass}>
              Torrents
            </NavLink>
            <NavLink to="/settings" className={linkClass}>
              Settings
            </NavLink>
          </nav>
        </div>
        <div className="flex-none">
          <button
            className="btn btn-ghost btn-sm"
            onClick={() => api.logout().then(() => navigate("/login"))}
          >
            Log out
          </button>
        </div>
      </div>
      <main className="mx-auto max-w-5xl p-4">
        <Outlet />
      </main>
    </div>
  );
}
