import { ReactNode } from "react";

type Accent = "default" | "primary" | "success" | "error";

interface StatCardProps {
  label: string;
  value: ReactNode;
  hint?: ReactNode;
  accent?: Accent;
}

const VALUE_ACCENT: Record<Accent, string> = {
  default: "",
  primary: "text-primary",
  success: "text-success",
  error: "text-error",
};

// A single dashboard metric tile.
export function StatCard({ label, value, hint, accent = "default" }: StatCardProps) {
  return (
    <div className="surface p-4">
      <div className="text-xs font-medium uppercase tracking-wide opacity-60">
        {label}
      </div>
      <div className={`mt-2 text-2xl font-bold tabular-nums ${VALUE_ACCENT[accent]}`}>
        {value}
      </div>
      {hint && <div className="mt-1 text-xs opacity-50">{hint}</div>}
    </div>
  );
}
