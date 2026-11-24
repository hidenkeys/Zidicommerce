import { useEffect, useMemo, useRef, useState } from "react";
import {
  ClipboardPaste,
  AlertCircle,
  MessageSquare,
  ShoppingBag,
  HelpCircle,
  Flag,
  Copy,
  GitBranch,
  Link2,
  Map as MapIcon,
  Maximize2,
  Minus,
  Plus,
  Wand2,
} from "lucide-react";

export type Point = { x: number; y: number };
type Node = {
  locked?: boolean;
  key: string;
  title: string;
  kind: string;
  next: string;
  failure: string;
  options?: { label: string; next?: string }[];
  position?: Point;
};
const width = 196,
  height = 76;
function layout(nodes: Node[], start: string) {
  const levels = new Map<string, number>();
  const byKey = new Map(nodes.map((node) => [node.key, node]));
  const queue = [start];
  levels.set(start, 0);
  for (let i = 0; i < queue.length; i++) {
    const node = byKey.get(queue[i]);
    if (!node) continue;
    for (const target of [
      node.next,
      ...(node.options ?? []).map((o) => o.next),
      node.failure,
    ]) {
      if (target && byKey.has(target) && !levels.has(target)) {
        levels.set(target, (levels.get(node.key) ?? 0) + 1);
        queue.push(target);
      }
    }
  }
  const rows = new Map<number, number>();
  const last = Math.max(0, ...levels.values()) + 1;
  return Object.fromEntries(
    nodes.map((node) => {
      const level = levels.get(node.key) ?? last,
        row = rows.get(level) ?? 0;
      rows.set(level, row + 1);
      return [node.key, { x: 32 + row * 272, y: 40 + level * 136 }];
    }),
  );
}

export function JourneyCanvas({
  nodes,
  start,
  selected,
  editable,
  select,
  arrange,
  paste,
  connect,
  invalidKeys = [],
}: {
  nodes: Node[];
  invalidKeys?: string[];
  start: string;
  selected: string;
  editable: boolean;
  paste: (keys: string[]) => void;
  connect: (target: string) => void;
  select: (id: string) => void;
  arrange: (positions: Record<string, Point>) => void;
}) {
  const auto = useMemo(() => layout(nodes, start), [nodes, start]);
  const [multiple, setMultiple] = useState(false);
  const [picked, setPicked] = useState<string[]>([]);
  const [copied, setCopied] = useState<string[]>([]);
  const [linking, setLinking] = useState(false);
  const [allFallbacks, setAllFallbacks] = useState(false);
  const [mapOpen, setMapOpen] = useState(false);
  const [view, setView] = useState({ x: 0, y: 0, w: 0, h: 0 });
  const [zoom, setZoom] = useState(1);
  const [drag, setDrag] = useState<{
    key: string;
    origin: Point;
    mouse: Point;
    at: Point;
  }>();
  const pan = useRef<{ mouse: Point; x: number; y: number }>();
  const viewport = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const el = viewport.current;
    if (!el) return;
    const update = () =>
      setView({
        x: el.scrollLeft / zoom,
        y: el.scrollTop / zoom,
        w: el.clientWidth / zoom,
        h: el.clientHeight / zoom,
      });
    const observer = new ResizeObserver(update);
    observer.observe(el);
    el.addEventListener("scroll", update, { passive: true });
    update();
    return () => {
      observer.disconnect();
      el.removeEventListener("scroll", update);
    };
  }, [zoom]);
  const group = picked.length ? picked : [selected];
  const canCopy =
    editable &&
    group.length > 0 &&
    group.every((key) => nodes.some((n) => n.key === key && !n.locked));
  const canConnect =
    editable &&
    nodes.some(
      (n) =>
        n.key === selected && !n.locked && !["handoff", "end"].includes(n.kind),
    );
  function choose(key: string, multiple: boolean) {
    if (linking) {
      if (key !== selected) {
        connect(key);
        setLinking(false);
      }
      return;
    }
    if (multiple) {
      setPicked((prev) => {
        const base = prev.length ? prev : [selected];
        return base.includes(key)
          ? base.filter((i) => i !== key)
          : [...base, key];
      });
      return;
    }
    setPicked([]);
    select(key);
  }
  const points = Object.fromEntries(
    nodes.map((node) => [
      node.key,
      drag?.key === node.key ? drag.at : (node.position ?? auto[node.key]),
    ]),
  );
  const w = Math.max(
    600,
    ...Object.values(points).map((p) => p.x + width + 48),
  );
  const h = Math.max(
    360,
    ...Object.values(points).map((p) => p.y + height + 48),
  );
  return (
    <section
      className="flow-canvas"
      aria-label="Workflow canvas"
      onKeyDown={(e) => {
        if (e.key === "Escape") {
          setLinking(false);
          setPicked([]);
          setMultiple(false);
        }
        if (!(e.metaKey || e.ctrlKey)) return;
        if (e.key.toLowerCase() === "c" && canCopy) {
          e.preventDefault();
          setCopied([...group]);
        }
        if (e.key.toLowerCase() === "v" && editable && copied.length) {
          e.preventDefault();
          paste(copied);
        }
      }}
    >
      <header>
        <h2>
          <GitBranch size={16} />
          Workflow
        </h2>
        <div>
          <button
            aria-label="Zoom out"
            disabled={zoom <= 0.2}
            onClick={() => setZoom(Math.max(0.2, zoom - 0.15))}
          >
            <Minus size={15} />
          </button>
          <output>{Math.round(zoom * 100)}%</output>
          <button
            aria-label="Zoom in"
            disabled={zoom >= 2}
            onClick={() => setZoom(Math.min(2, zoom + 0.15))}
          >
            <Plus size={15} />
          </button>
          <button
            aria-label="Fit workflow to view"
            onClick={() => {
              if (viewport.current) {
                setZoom(
                  Math.min(
                    1,
                    viewport.current.clientWidth / w,
                    viewport.current.clientHeight / h,
                  ),
                );
                viewport.current.scrollTo(0, 0);
              }
            }}
          >
            <Maximize2 size={15} />
          </button>
          {editable && (
            <button
              aria-label="Auto-layout workflow"
              onClick={() => arrange(auto)}
            >
              <Wand2 size={15} />
            </button>
          )}
          <button
            aria-label="Show all fallback routes"
            title="Show all fallback routes"
            aria-pressed={allFallbacks}
            onClick={() => setAllFallbacks(!allFallbacks)}
          ><GitBranch size={15}/></button>
          <button
            aria-label="Toggle minimap"
            aria-pressed={mapOpen}
            onClick={() => setMapOpen(!mapOpen)}
          >
            <MapIcon size={15} />
          </button>
        </div>
      </header>
      {editable && (
        <div className="flow-edit-tools">
          <button
            disabled={!canConnect}
            aria-pressed={linking}
            onClick={() => setLinking(!linking)}
          >
            <Link2 size={14} />
            {linking ? "Choose next step" : "Connect next"}
          </button>
          <button disabled={!canCopy} onClick={() => setCopied([...group])}>
            <Copy size={14} />
            Copy{group.length > 1 ? " " + group.length : ""}
          </button>
          <button disabled={!copied.length} onClick={() => paste(copied)}>
            <ClipboardPaste size={14} />
            Paste
          </button>
          <button
            aria-pressed={multiple}
            onClick={() => {
              setMultiple(!multiple);
              if (multiple) setPicked([]);
            }}
          >
            Select multiple
          </button>
        </div>
      )}
      {mapOpen && (
        <svg
          className="flow-minimap"
          role="img"
          aria-label="Workflow minimap"
          viewBox={`0 0 ${w} ${h}`}
        >
          {nodes.map((n) => (
            <rect
              key={n.key}
              x={points[n.key].x}
              y={points[n.key].y}
              width={width}
              height={height}
              rx={8}
              fill={n.key === selected ? "var(--z-focus)" : "var(--z-line)"}
            />
          ))}
          <rect
            x={view.x}
            y={view.y}
            width={Math.min(w, view.w)}
            height={Math.min(h, view.h)}
            fill="none"
            stroke="var(--z-line)"
            strokeWidth={8}
          />
        </svg>
      )}
      <div
        className="flow-viewport"
        ref={viewport}
        tabIndex={0}
        aria-label="Scrollable workflow"
        onPointerDown={(e) => {
          if (e.button || (e.target as Element).closest("button")) return;
          pan.current = {
            mouse: { x: e.clientX, y: e.clientY },
            x: e.currentTarget.scrollLeft,
            y: e.currentTarget.scrollTop,
          };
          e.currentTarget.setPointerCapture(e.pointerId);
        }}
        onPointerMove={(e) => {
          const p = pan.current;
          if (p)
            e.currentTarget.scrollTo(
              p.x + p.mouse.x - e.clientX,
              p.y + p.mouse.y - e.clientY,
            );
        }}
        onLostPointerCapture={() => {
          pan.current = undefined;
        }}
      >
        <div
          style={{
            width: w * zoom,
            height: h * zoom,
            position: "relative",
            overflow: "hidden",
          }}
        >
          <div
            style={{
              width: w,
              height: h,
              position: "absolute",
              transformOrigin: "top left",
              transform: "scale(" + zoom + ")",
            }}
          >
            <svg width={w} height={h} aria-hidden="true" className="flow-links">
              <defs>
                <marker
                  id="workflow-arrow"
                  viewBox="0 0 10 10"
                  refX="9"
                  refY="5"
                  markerWidth="6"
                  markerHeight="6"
                  orient="auto-start-reverse"
                >
                  <path d="M0 0L10 5L0 10" fill="var(--z-line)" />
                </marker>
              </defs>
              {nodes.flatMap((node) =>
                [
                  {
                    id: node.next,
                    label: node.kind === "condition" ? "Yes" : "",
                  },
                  ...(node.options ?? []).map((o) => ({
                    id: o.next,
                    label: o.label,
                  })),
                  {
                    id: node.failure,
                    label: node.kind === "condition" ? "No" : "Fallback",
                  },
                ].map((edge, index) => {
                  const a = points[node.key],
                    b = edge.id && points[edge.id];
                  if (!b) return null;
                  if (edge.label === "Fallback" && !allFallbacks && node.key !== selected) return null;
                  const downward = b.y >= a.y + height;
                  const sideways = Math.abs(b.x - a.x) > Math.abs(b.y - a.y);
                  const from = sideways ? {x:a.x + width,y:a.y + height/2} : {x:a.x + width/2,y:a.y + (downward?height:0)};
                  const to = sideways ? {x:b.x,y:b.y + height/2} : {x:b.x + width/2,y:b.y + (downward?0:height)};
                  const bend = Math.min(80,Math.max(24,(sideways ? Math.abs(to.x-from.x) : Math.abs(to.y-from.y))/2));
                  const offset = downward ? bend : -bend;
                  const curve = sideways
                    ? `M${from.x} ${from.y} C${from.x+bend} ${from.y} ${to.x-bend} ${to.y} ${to.x} ${to.y}`
                    : `M${from.x} ${from.y} C${from.x} ${from.y+offset} ${to.x} ${to.y-offset} ${to.x} ${to.y}`;
                  return (
                    <g key={node.key + ":" + index}>
                      <path
                        d={curve}
                        fill="none"
                        stroke="var(--z-line)"
                        strokeDasharray={
                          edge.label === "Fallback" ? "4 4" : undefined
                        }
                        markerEnd="url(#workflow-arrow)"
                      />
                      {edge.label && (
                        <text
                          x={(from.x + to.x) / 2}
                          y={(from.y + to.y) / 2 - 5}
                          textAnchor="middle"
                        >
                          {edge.label}
                        </text>
                      )}
                    </g>
                  );
                }),
              )}
            </svg>
            {nodes.map((node) => (
              <button
                type="button"
                key={node.key}
                className="flow-node"
                data-invalid={invalidKeys.includes(node.key)}
                aria-invalid={invalidKeys.includes(node.key)}
                aria-pressed={group.includes(node.key)}
                style={{
                  left: points[node.key].x,
                  top: points[node.key].y,
                  width,
                  height,
                }}
                onClick={(e) => choose(node.key, e.shiftKey || multiple)}
                onKeyDown={(e) => {
                  if (
                    !editable ||
                    !e.altKey ||
                    ![
                      "ArrowLeft",
                      "ArrowRight",
                      "ArrowUp",
                      "ArrowDown",
                    ].includes(e.key)
                  )
                    return;
                  e.preventDefault();
                  const pos = points[node.key];
                  arrange({
                    [node.key]: {
                      x: Math.max(
                        0,
                        Math.min(
                          10000,
                          pos.x +
                            (e.key === "ArrowRight"
                              ? 16
                              : e.key === "ArrowLeft"
                                ? -16
                                : 0),
                        ),
                      ),
                      y: Math.max(
                        0,
                        Math.min(
                          10000,
                          pos.y +
                            (e.key === "ArrowDown"
                              ? 16
                              : e.key === "ArrowUp"
                                ? -16
                                : 0),
                        ),
                      ),
                    },
                  });
                }}
                onPointerDown={(e) => {
                  if (!editable || e.button) return;
                  const at = points[node.key];
                  e.currentTarget.setPointerCapture(e.pointerId);
                  setDrag({
                    key: node.key,
                    origin: at,
                    mouse: { x: e.clientX, y: e.clientY },
                    at,
                  });
                }}
                onPointerMove={(e) => {
                  if (drag?.key !== node.key) return;
                  const snap = (n: number) =>
                    Math.min(10000, Math.max(0, Math.round(n / 16) * 16));
                  setDrag({
                    ...drag,
                    at: {
                      x: snap(
                        drag.origin.x + (e.clientX - drag.mouse.x) / zoom,
                      ),
                      y: snap(
                        drag.origin.y + (e.clientY - drag.mouse.y) / zoom,
                      ),
                    },
                  });
                }}
                onPointerUp={(e) => {
                  if (
                    drag?.key === node.key &&
                    Math.hypot(
                      e.clientX - drag.mouse.x,
                      e.clientY - drag.mouse.y,
                    ) > 4
                  )
                    arrange({ [node.key]: drag.at });
                  setDrag(undefined);
                }}
                onPointerCancel={() => setDrag(undefined)}
              >
                <small>
                  {node.kind === "condition" ? <GitBranch size={13}/> : ["end", "handoff"].includes(node.kind) ? <Flag size={13}/> : node.kind.includes("selector") || ["payment", "create_order"].includes(node.kind) ? <ShoppingBag size={13}/> : ["question", "buttons", "list"].includes(node.kind) ? <HelpCircle size={13}/> : <MessageSquare size={13}/>}
                  {node.key === start ? "Start · " : ""}
                  {node.kind.replace(/_/g, " ")}
                </small>
                <strong>{node.title}</strong>
                {invalidKeys.includes(node.key) && <small className="node-validation"><AlertCircle size={12}/>Needs attention</small>}
              </button>
            ))}
          </div>
        </div>
      </div>
    </section>
  );
}
