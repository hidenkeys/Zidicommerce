import { forwardRef, useEffect, useId, useRef, useState, type ButtonHTMLAttributes, type FormEvent, type HTMLAttributes, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { Link } from "react-router-dom";
import { AlertTriangle, ArrowUpRight, Check, ChevronRight, Inbox, LoaderCircle, Search, X, type LucideIcon } from "lucide-react";

export function Page({
  title,
  description,
  help,
  actions,
  breadcrumbs,
  eyebrow,
  children,
}: {
  title: string;
  eyebrow?: string;
  description?: string;
  help?: string;
  actions?: ReactNode;
  breadcrumbs?: Array<{ label: string; to?: string }>;
  children: ReactNode;
}) {
  return (
    <section className="content" data-page={title.toLowerCase().replace(/[^a-z0-9]+/g, "-")}>
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
          {eyebrow ? <span className="page-eyebrow">{eyebrow}</span> : null}
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
  icon: Icon = Inbox,
}: {
  title: string;
  body?: string;
  action?: ReactNode;
  icon?: LucideIcon;
}) {
  return (
    <div className="empty-state">
      <span className="empty-state__art" aria-hidden="true"><Icon size={22} /><i /></span>
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
      {items.map((item) => <button key={item.value} type="button" role="tab" tabIndex={value === item.value ? 0 : -1} onKeyDown={(event) => { const current = items.findIndex(entry => entry.value === item.value); const next = event.key === "ArrowRight" ? (current + 1) % items.length : event.key === "ArrowLeft" ? (current - 1 + items.length) % items.length : event.key === "Home" ? 0 : event.key === "End" ? items.length - 1 : -1; if (next >= 0) { event.preventDefault(); onChange(items[next].value); (event.currentTarget.parentElement?.children[next] as HTMLElement)?.focus(); } }} aria-selected={value === item.value} className={value === item.value ? "active" : ""} onClick={() => onChange(item.value)}>{item.label}{item.count !== undefined ? <span>{item.count}</span> : null}</button>)}
    </div>
  );
}

function useDialogFocus(open: boolean, onClose: () => void) {
  const closeRef = useRef<HTMLButtonElement>(null);
  const callback = useRef(onClose);
  callback.current = onClose;
  useEffect(() => {
    if (!open) return;
    const previous = document.activeElement as HTMLElement | null;
    const overflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    closeRef.current?.focus();
    function onKey(event: KeyboardEvent) {
      const dialog = closeRef.current?.closest('[role="dialog"]');
      const dialogs = document.querySelectorAll('[role="dialog"]');
      if (!dialog || dialog !== dialogs[dialogs.length - 1]) return;
      if (event.key === "Escape") { event.preventDefault(); callback.current(); }
      if (event.key !== "Tab") return;
      const focusable = Array.from(dialog.querySelectorAll<HTMLElement>('a[href],button,input,select,textarea,[tabindex]')).filter(el => el.tabIndex >= 0 && !el.matches(":disabled") && el.getClientRects().length);
      const first = focusable[0], last = focusable[focusable.length - 1];
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus(); }
      else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus(); }
    }
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("keydown", onKey);
      document.body.style.overflow = overflow;
      if (previous?.isConnected) previous.focus();
    };
  }, [open]);
  return closeRef;
}

export function Modal({ open, title, description, onClose, children, footer, danger = false }: { open: boolean; title: string; description?: string; onClose: () => void; children?: ReactNode; footer?: ReactNode; danger?: boolean }) {
  const titleID = useId();
  const descriptionID = useId();
  const closeRef = useDialogFocus(open, onClose);
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
  const closeRef = useDialogFocus(open, onClose);

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

export type MetricTone = "brand" | "success" | "info" | "neutral" | "warning";

export function Metric({
  label,
  value,
  hint,
  to,
  icon: Icon,
  tone = "neutral",
  prefix,
  dot,
}: {
  label: string;
  value: ReactNode;
  hint?: string;
  to?: string;
  icon?: LucideIcon;
  tone?: MetricTone;
  prefix?: string;
  dot?: "success" | "warning" | "danger" | "info" | "neutral";
}) {
  const body = (
    <>
      <span className="metric-card__label">
        {Icon ? <span className={`metric-card__icon tone-${tone}`} aria-hidden="true"><Icon size={16} /></span> : null}
        {label}
      </span>
      <strong>{prefix ? <small className="metric-card__prefix">{prefix}</small> : null}{value}</strong>
      {hint ? <p>{dot ? <i className={`metric-card__dot tone-${dot}`} aria-hidden="true" /> : null}{hint}{to ? <ArrowUpRight className="metric-card__go" size={14} aria-hidden="true" /> : null}</p> : null}
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

/** Connected stat cells: four across, two-by-two on medium widths, one column on phones. */
export function MetricStrip({ children, label, className = "" }: { children: ReactNode; label?: string; className?: string }) {
  return <div className={`metric-strip ${className}`.trim()} role={label ? "group" : undefined} aria-label={label}>{children}</div>;
}

export function FormGrid({ children, onSubmit, title }: { children: ReactNode; onSubmit?: (event: FormEvent) => void | Promise<void>; title?: string }) {
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  async function submit(event: FormEvent) {
    if (!onSubmit) return;
    event.preventDefault();
    if (saving) return;
    setSaving(true); setError("");
    try { await onSubmit(event); }
    catch (reason) { setError(reason instanceof Error ? reason.message : "Changes could not be saved. Please try again."); }
    finally { setSaving(false); }
  }
  return (
    <form className="form-grid" onSubmit={submit} aria-busy={saving}>
      {title ? <strong>{title}</strong> : null}
      {error ? <p className="flash flash-error full" role="alert">{error}</p> : null}
      <fieldset className="form-fields" disabled={saving}>{children}</fieldset>
      {saving ? <span className="full muted" role="status">Saving changes…</span> : null}
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
