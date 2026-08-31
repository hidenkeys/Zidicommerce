import { forwardRef, useEffect, useId, useRef, useState, type ButtonHTMLAttributes, type FormEvent, type HTMLAttributes, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { Link } from "react-router-dom";
import { AlertTriangle, Check, ChevronRight, LoaderCircle, Search, X, type LucideIcon } from "lucide-react";

export function Page({
  title,
  description,
  help,
  actions,
  breadcrumbs,
  children,
}: {
  title: string;
  description?: string;
  help?: string;
  actions?: ReactNode;
  breadcrumbs?: Array<{ label: string; to?: string }>;
  children: ReactNode;
}) {
  return (
    <section className="content">
      {breadcrumbs?.length ? (
        <nav className="breadcrumbs" aria-label="Breadcrumb">
          {breadcrumbs.map((item, index) => (
            <span key={`${item.label}-${index}`}>
              {index > 0 ? <ChevronRight size={14} aria-hidden="true" /> : null}
              {item.to ? <Link to={item.to}>{item.label}</Link> : <span aria-current="page">{item.label}</span>}
            </span>
          ))}
        </nav>
      ) : null}
      <div className="page-header">
        <div>
          <h1>{title}</h1>
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
  const [visible, setVisible] = useState(Boolean(message));
  useEffect(() => {
    setVisible(Boolean(message));
    if (!message || tone !== "success") return;
    const timer = window.setTimeout(() => setVisible(false), 4500);
    return () => window.clearTimeout(timer);
  }, [message, tone]);
  if (!message || !visible) return null;
  return <p className={`flash flash-${tone}`} role={tone === "error" ? "alert" : "status"}>{message}</p>;
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
    <div className="loading-state" role="status" aria-live="polite">
      <LoaderCircle className="spin" size={20} aria-hidden="true" />
      <div>
        <strong>{label}</strong>
        <span>This should only take a moment.</span>
      </div>
    </div>
  );
}

export function SkeletonRows({ rows = 4 }: { rows?: number }) {
  return (
    <div className="skeleton-list" aria-label="Loading content" role="status">
      {Array.from({ length: rows }, (_, index) => <span key={index} className="skeleton-row" />)}
    </div>
  );
}

export function StatusDot({ live, label }: { live: boolean; label: string }) {
  return (
    <span className={`status-dot ${live ? "live" : "offline"}`}>
      <i aria-hidden="true" />
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

export function Card({ children, className = "", ...props }: { children: ReactNode; className?: string } & HTMLAttributes<HTMLElement>) {
  return <article className={`panel ${className}`} {...props}>{children}</article>;
}

export function SectionHeader({ title, description, action }: { title: string; description?: string; action?: ReactNode }) {
  return (
    <div className="subsection-header">
      <div><h2>{title}</h2>{description ? <p>{description}</p> : null}</div>
      {action ? <div className="page-actions">{action}</div> : null}
    </div>
  );
}

export function Button({ icon: Icon, loading, children, className = "", ...props }: ButtonHTMLAttributes<HTMLButtonElement> & { icon?: LucideIcon; loading?: boolean }) {
  return (
    <button {...props} className={className} disabled={props.disabled || loading}>
      {loading ? <LoaderCircle className="spin" size={16} aria-hidden="true" /> : Icon ? <Icon size={16} aria-hidden="true" /> : null}
      <span>{children}</span>
    </button>
  );
}

export const IconButton = forwardRef<HTMLButtonElement, Omit<ButtonHTMLAttributes<HTMLButtonElement>, "children"> & { label: string; icon: LucideIcon }>(function IconButton({ label, icon: Icon, className = "", ...props }, ref) {
  return <button {...props} ref={ref} type={props.type ?? "button"} className={`icon-button ${className}`} aria-label={label} title={label}><Icon size={17} aria-hidden="true" /></button>;
});

export function Tabs({ value, onChange, items, label }: { value: string; onChange: (value: string) => void; items: Array<{ value: string; label: string; count?: number }>; label: string }) {
  return (
    <div className="tabs" role="tablist" aria-label={label}>
      {items.map((item) => <button key={item.value} type="button" role="tab" aria-selected={value === item.value} className={value === item.value ? "active" : ""} onClick={() => onChange(item.value)}>{item.label}{item.count !== undefined ? <span>{item.count}</span> : null}</button>)}
    </div>
  );
}

export function Modal({ open, title, description, onClose, children, footer, danger = false }: { open: boolean; title: string; description?: string; onClose: () => void; children?: ReactNode; footer?: ReactNode; danger?: boolean }) {
  const titleID = useId();
  const descriptionID = useId();
  const closeRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (!open) return;
    const previous = document.activeElement as HTMLElement | null;
    const overflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    closeRef.current?.focus();
    function onKey(event: KeyboardEvent) {
      if (event.key === "Escape") onClose();
    }
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("keydown", onKey);
      document.body.style.overflow = overflow;
      previous?.focus();
    };
  }, [open, onClose]);
  if (!open) return null;
  return createPortal(
    <div className="modal-backdrop" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <section className="modal" role="dialog" aria-modal="true" aria-labelledby={titleID} aria-describedby={description ? descriptionID : undefined}>
        <div className="modal-head">
          <div className={danger ? "modal-symbol danger" : "modal-symbol"}>{danger ? <AlertTriangle size={20} aria-hidden="true" /> : <Check size={20} aria-hidden="true" />}</div>
          <div><h2 id={titleID}>{title}</h2>{description ? <p id={descriptionID}>{description}</p> : null}</div>
          <IconButton ref={closeRef} label="Close dialog" icon={X} onClick={onClose} />
        </div>
        {children ? <div className="modal-body">{children}</div> : null}
        {footer ? <div className="modal-footer">{footer}</div> : null}
      </section>
    </div>,
    document.body,
  );
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
  const titleID = useId();
  const closeRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (!open) return;
    const previous = document.activeElement as HTMLElement | null;
    const overflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    closeRef.current?.focus();
    function onKey(event: KeyboardEvent) {
      if (event.key === "Escape") onClose();
    }
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("keydown", onKey);
      document.body.style.overflow = overflow;
      previous?.focus();
    };
  }, [open, onClose]);

  if (!open) return null;
  return createPortal(
    <div className="drawer-backdrop" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <aside className="drawer" role="dialog" aria-modal="true" aria-labelledby={titleID}>
        <div className="drawer-head">
          <h2 id={titleID}>{title}</h2>
          <IconButton ref={closeRef} label="Close panel" icon={X} onClick={onClose} />
        </div>
        {children}
      </aside>
    </div>,
    document.body,
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
  const [open, setOpen] = useState(false);
  return (
    <>
      <button type="button" className={tone === "danger" ? "danger" : undefined} onClick={() => setOpen(true)}>{label}</button>
      <Modal open={open} title={label} description={confirm} danger={tone === "danger"} onClose={() => setOpen(false)} footer={<><button type="button" className="ghost" onClick={() => setOpen(false)}>Cancel</button><button type="button" className={tone === "danger" ? "danger" : "primary"} onClick={() => { setOpen(false); onConfirm(); }}>{label}</button></>} />
    </>
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
    <label className="search-control">
      <Search size={16} aria-hidden="true" />
      <span className="sr-only">{placeholder}</span>
      <input className="search-field" value={value} placeholder={placeholder} onChange={(event) => onChange(event.target.value)} />
    </label>
  );
}

export function FilterBar({ children }: { children: ReactNode }) {
  return <div className="filter-bar" role="search">{children}</div>;
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
