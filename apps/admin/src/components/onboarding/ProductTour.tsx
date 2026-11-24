import { useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import type { TourStep, TourFlag } from "../../lib/onboarding";
import "./onboarding.css";

export function ProductTour({ steps, onClose }: { steps: TourStep[]; onClose: (flag: TourFlag) => void }) {
  const [index, setIndex] = useState(0);
  const [rect, setRect] = useState<DOMRect | null>(null);
  const dialog = useRef<HTMLDivElement>(null);
  const previousFocus = useRef(document.activeElement as HTMLElement | null);
  const navigate = useNavigate();
  const step = steps[index];
  useEffect(() => () => { if (previousFocus.current?.isConnected) previousFocus.current.focus(); }, []);
  useEffect(() => {
    dialog.current?.focus();
    const measure = () => {
      const anchor = document.querySelector<HTMLElement>(`[data-tour="${step.id}"]`);
      const bounds = anchor?.getBoundingClientRect();
      setRect(bounds && bounds.width > 0 && bounds.top >= 0 && bounds.bottom <= window.innerHeight && bounds.left >= 0 && bounds.right <= window.innerWidth && window.innerWidth >= 768 ? bounds : null);
    };
    measure();
    window.addEventListener("resize", measure);
    window.addEventListener("scroll", measure, true);
    return () => { window.removeEventListener("resize", measure); window.removeEventListener("scroll", measure, true); };
  }, [step.id]);
  function finish(flag: TourFlag) { onClose(flag); if (flag === "completed" && step.id === "setup") navigate("/setup"); }
  return <div className="product-tour">
    <div className={rect ? "tour-shade clear" : "tour-shade"} aria-hidden="true" />
    {rect && <div className="tour-spotlight" aria-hidden="true" style={{ top: rect.top, left: rect.left, width: rect.width, height: rect.height }} />}
    <div ref={dialog} tabIndex={-1} role="dialog" aria-modal="true" aria-labelledby="tour-title" aria-describedby="tour-body" className={`tour-card${rect ? " anchored" : ""}`} style={rect ? { left: Math.min(rect.right + 16, window.innerWidth - 376), top: Math.max(16, Math.min(rect.top, window.innerHeight - 300)) } : undefined} onKeyDown={event => {
      if (event.key === "Escape") { event.preventDefault(); finish("dismissed"); }
      if (event.key === "Tab") {
        const buttons = dialog.current?.querySelectorAll<HTMLButtonElement>("button:not(:disabled)");
        if (!buttons?.length) return;
        const first = buttons[0], last = buttons[buttons.length - 1];
        if (event.shiftKey && (document.activeElement === first || document.activeElement === dialog.current)) { event.preventDefault(); last.focus(); }
        else if (!event.shiftKey && (document.activeElement === last || document.activeElement === dialog.current)) { event.preventDefault(); first.focus(); }
      }
    }}>
      <small aria-live="polite">Step {index + 1} of {steps.length}</small>
      <h2 id="tour-title">{step.title}</h2><p id="tour-body">{step.body}</p>
      <div className="tour-actions"><button type="button" onClick={() => finish("dismissed")}>Skip tour</button><button type="button" disabled={index === 0} onClick={() => setIndex(i => i - 1)}>Back</button><button type="button" className="primary" onClick={() => index === steps.length - 1 ? finish("completed") : setIndex(i => i + 1)}>{index === steps.length - 1 ? "Finish" : "Next"}</button></div>
    </div>
  </div>;
}
