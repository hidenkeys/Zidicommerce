import { FormEvent, ReactNode, useEffect } from "react";
import { Link } from "react-router-dom";

export function Page({
  title,
  description,
  help,
  actions,
  children,
}: {
  title: string;
  description?: string;
  help?: string;
  actions?: ReactNode;
  children: ReactNode;
}) {
  return (
    <section className="content">
      <div className="page-header">
        <div>
          <h2>{title}</h2>
          {description ? <p>{description}</p> : null}
          {help ? <p className="help-text">{help}</p> : null}
        </div>
        {actions ? <div className="page-actions">{actions}</div> : null}
      </div>
      {children}
    </section>
  );
}

export function Flash({ message, tone = "error" }: { message?: string; tone?: "error" | "success" | "info" }) {
  if (!message) return null;
  return <p className={`flash flash-${tone}`}>{message}</p>;
}

export function EmptyState({
  title,
  body,
  action,
}: {
  title: string;
  body?: string;
  action?: ReactNode;
}) {
  return (
    <div className="empty-state">
      <strong>{title}</strong>
      {body ? <span>{body}</span> : null}
      {action}
    </div>
  );
}

export function LoadingState({ label = "Loading" }: { label?: string }) {
  return (
    <div className="empty-state">
      <strong>{label}</strong>
      <span>This should only take a moment.</span>
    </div>
  );
}

export function StatusDot({ live, label }: { live: boolean; label: string }) {
  return (
    <span className={`status-dot ${live ? "live" : "offline"}`}>
      <i />
      {label}
    </span>
  );
}

export function Badge({ children, tone = "neutral" }: { children: ReactNode; tone?: "neutral" | "success" | "warning" | "danger" | "info" }) {
  return <span className={`badge tone-${tone}`}>{children}</span>;
}

export function Help({ children }: { children: ReactNode }) {
  return <p className="help-text">{children}</p>;
}

export function Card({ children, className = "" }: { children: ReactNode; className?: string }) {
  return <article className={`panel ${className}`}>{children}</article>;
}

export function Drawer({
  open,
  title,
  onClose,
  children,
}: {
  open: boolean;
  title: string;
  onClose: () => void;
  children: ReactNode;
}) {
  useEffect(() => {
    if (!open) return;
    function onKey(event: KeyboardEvent) {
      if (event.key === "Escape") onClose();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onClose]);

  if (!open) return null;
  return (
    <div className="drawer-backdrop" onClick={onClose}>
      <aside className="drawer" onClick={(event) => event.stopPropagation()}>
        <div className="drawer-head">
          <h3>{title}</h3>
          <button type="button" className="ghost" onClick={onClose}>
            Close
          </button>
        </div>
        {children}
      </aside>
    </div>
  );
}

export function ConfirmButton({
  label,
  confirm,
  onConfirm,
  tone = "default",
}: {
  label: string;
  confirm: string;
  onConfirm: () => void;
  tone?: "default" | "danger";
}) {
  return (
    <button
      type="button"
      className={tone === "danger" ? "danger" : undefined}
      onClick={() => {
        if (window.confirm(confirm)) onConfirm();
      }}
    >
      {label}
    </button>
  );
}

export function SearchField({
  value,
  onChange,
  placeholder,
}: {
  value: string;
  onChange: (value: string) => void;
  placeholder: string;
}) {
  return (
    <input
      className="search-field"
      value={value}
      placeholder={placeholder}
      onChange={(event) => onChange(event.target.value)}
    />
  );
}

export function FilterBar({ children }: { children: ReactNode }) {
  return <div className="filter-bar">{children}</div>;
}

export function Metric({
  label,
  value,
  hint,
  to,
}: {
  label: string;
  value: ReactNode;
  hint?: string;
  to?: string;
}) {
  const body = (
    <>
      <span>{label}</span>
      <strong>{value}</strong>
      {hint ? <p>{hint}</p> : null}
    </>
  );
  if (to) {
    return (
      <Link className="metric-card" to={to}>
        {body}
      </Link>
    );
  }
  return <article className="metric-card">{body}</article>;
}

export function FormGrid({ children, onSubmit, title }: { children: ReactNode; onSubmit?: (event: FormEvent) => void; title?: string }) {
  return (
    <form className="form-grid" onSubmit={onSubmit}>
      {title ? <strong>{title}</strong> : null}
      {children}
    </form>
  );
}

export function SetupItem({
  complete,
  label,
  to,
  description,
}: {
  complete: boolean;
  label: string;
  to?: string;
  description?: string;
}) {
  const content = (
    <>
      <span className={complete ? "check done" : "check"}>{complete ? "✓" : ""}</span>
      <span>
        <strong>{label}</strong>
        {description ? <em>{description}</em> : null}
      </span>
    </>
  );
  if (to) {
    return (
      <Link className={`setup-item ${complete ? "complete" : ""}`} to={to}>
        {content}
      </Link>
    );
  }
  return <div className={`setup-item ${complete ? "complete" : ""}`}>{content}</div>;
}

export function LearnMore({ href, children }: { href: string; children: ReactNode }) {
  return (
    <Link className="learn-more" to={href}>
      {children}
    </Link>
  );
}
