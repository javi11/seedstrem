import { useLocation } from "react-router-dom";
import { useNavigationGuard } from "./NavigationGuard";

interface NavItemProps {
  to: string;
  icon: string;
  label: string;
  end?: boolean;
  onNavigate?: () => void;
}

// A single sidebar navigation entry. Routes through the navigation guard so an
// unsaved-changes prompt can intercept the move. Active state gets a base-300
// wash and a left accent bar.
export function NavItem({ to, icon, label, end, onNavigate }: NavItemProps) {
  const { pathname } = useLocation();
  const { requestNavigate } = useNavigationGuard();
  const isActive = end ? pathname === to : pathname.startsWith(to);

  return (
    <button
      onClick={() => {
        onNavigate?.();
        requestNavigate(to);
      }}
      aria-current={isActive ? "page" : undefined}
      className={[
        "group flex items-center gap-3 rounded-field px-3 py-2.5 text-left text-sm font-medium transition-colors",
        isActive
          ? "bg-base-300 text-base-content shadow-[inset_2px_0_0_0_var(--color-primary)]"
          : "text-base-content/60 hover:bg-base-300/60 hover:text-base-content",
      ].join(" ")}
    >
      <span className="w-5 text-center text-base opacity-90" aria-hidden>
        {icon}
      </span>
      {label}
    </button>
  );
}
