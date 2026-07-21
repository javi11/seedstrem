import { statusPresentation } from "../lib/status";

// Friendly, colour-coded status pill driven by the shared presentation map.
export function StatusBadge({ status }: { status: string }) {
  const p = statusPresentation(status);
  return (
    <span className={`badge ${p.badgeClass} gap-1 whitespace-nowrap font-medium`}>
      <span aria-hidden>{p.icon}</span>
      {p.label}
    </span>
  );
}
