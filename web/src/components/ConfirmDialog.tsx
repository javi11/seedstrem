import { ReactNode } from "react";

interface ConfirmDialogProps {
  open: boolean;
  title: string;
  children?: ReactNode;
  confirmLabel?: string;
  cancelLabel?: string;
  danger?: boolean;
  busy?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

// A small controlled confirm modal, reused for destructive deletes and the
// unsaved-changes guard. Built on daisyUI's <dialog> modal styling.
export function ConfirmDialog({
  open,
  title,
  children,
  confirmLabel = "Confirm",
  cancelLabel = "Cancel",
  danger = false,
  busy = false,
  onConfirm,
  onCancel,
}: ConfirmDialogProps) {
  if (!open) return null;

  return (
    <div className="modal modal-open" role="dialog" aria-modal="true">
      <div className="modal-box surface">
        <h3 className="text-lg font-bold tracking-brand">{title}</h3>
        {children && <div className="py-3 text-sm opacity-80">{children}</div>}
        <div className="modal-action">
          <button className="btn btn-ghost" onClick={onCancel} disabled={busy}>
            {cancelLabel}
          </button>
          <button
            className={`btn ${danger ? "btn-error" : "btn-primary"}`}
            onClick={onConfirm}
            disabled={busy}
          >
            {busy ? <span className="loading loading-spinner loading-sm" /> : confirmLabel}
          </button>
        </div>
      </div>
      <button
        className="modal-backdrop"
        aria-label="Close dialog"
        onClick={onCancel}
        disabled={busy}
      />
    </div>
  );
}
