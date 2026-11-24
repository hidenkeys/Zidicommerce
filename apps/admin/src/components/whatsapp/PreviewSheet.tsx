import { useEffect, useRef, type ReactNode } from "react";
import { X } from "lucide-react";

export function PreviewSheet({ title, children, onClose, full = false }: { title: string; children: ReactNode; onClose: () => void; full?: boolean }) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    const element = ref.current;
    element?.focus();
    function keydown(event: KeyboardEvent) {
      if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); onClose(); }
      if (event.key !== "Tab") return;
      const nodes = Array.from(element?.querySelectorAll<HTMLElement>('button:not(:disabled),input:not(:disabled),select:not(:disabled),textarea:not(:disabled),a[href],[tabindex="0"]') ?? []).filter(node => !node.hidden);
      const first = nodes[0], last = nodes[nodes.length - 1];
      if (!first) { event.preventDefault(); element?.focus(); }
      else if (event.shiftKey && (document.activeElement === first || document.activeElement === element)) { event.preventDefault(); last.focus(); }
      else if (!event.shiftKey && (document.activeElement === last || document.activeElement === element)) { event.preventDefault(); first.focus(); }
    }
    function containFocus(event: FocusEvent) {
      if (element && event.target instanceof Node && !element.contains(event.target)) element.focus();
    }
    element?.addEventListener("keydown", keydown);
    document.addEventListener("focusin", containFocus);
    return () => { element?.removeEventListener("keydown", keydown); document.removeEventListener("focusin", containFocus); previous?.focus(); };
  }, [onClose]);
  return <div className="wa-sheet-backdrop"><div ref={ref} tabIndex={-1} role="dialog" aria-modal="true" aria-label={title} className={`wa-sheet ${full ? "wa-sheet-full" : ""}`}><header><button type="button" aria-label={`Close ${title}`} onClick={onClose}><X size={20}/></button><strong>{title}</strong><span/></header>{children}</div></div>;
}
