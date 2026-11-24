import { useEffect, useRef, useState, useCallback } from "react";
import { CheckCheck, ImageIcon, List, MapPin, ExternalLink } from "lucide-react";
import { normalizeMessage } from "./model";
import { PreviewSheet } from "./PreviewSheet";
import "./whatsapp-preview.css";

type Line = { from: "customer" | "bot"; message: unknown; timestamp?: string };
type Reply = (id: string, label: string) => void;
export function WhatsAppMessage({ line, grouped = false, active = false, busy = false, onReply, onOpenFlow }: { line: Line; grouped?: boolean; active?: boolean; busy?: boolean; onReply?: Reply; onOpenFlow?: () => void }) {
  const message = normalizeMessage(line.message);
  const [listOpen, setListOpen] = useState(false);
  const close = useCallback(() => setListOpen(false), []);
  const outgoing = line.from === "customer";
  const canReply = active && !busy && !!onReply;
  const list = message.kind === "list" || message.options.length > 3;
  const replies = !list && message.options.length > 0;
  const time = line.timestamp ?? message.timestamp;
  const date = time ? new Date(time) : undefined;
  const timestamp = date && !Number.isNaN(date.getTime()) ? date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }) : "9:41";
  if (message.kind === "notice") return <p className="wa-notice">{message.text}</p>;
  return <div className={`wa-message ${outgoing ? "wa-outgoing" : "wa-incoming"} ${grouped ? "wa-grouped" : ""}`}>
    <div className="wa-bubble">
      {message.kind === "image" ? <div className="wa-media" role="img" aria-label={message.text || "Image preview"}><ImageIcon size={36}/><span>Image preview</span>{message.mediaUrl && <small>Media attachment</small>}</div> : null}
      <p>{message.text}</p>{message.help ? <small className="wa-help">{message.help}</small> : null}
      {message.kind === "fallback" && !message.text ? <p>This message cannot be displayed in the preview.</p> : null}
      <div className="wa-message-meta"><time dateTime={time}>{timestamp}</time>{outgoing && <CheckCheck size={15} aria-label="Read"/>}</div>
      {list ? <button className="wa-action" type="button" disabled={busy} onClick={() => setListOpen(true)}><List size={17}/>View options</button> : null}
      {message.url ? <a className="wa-action" href={message.url} target="_blank" rel="noopener noreferrer"><ExternalLink size={16}/>Open link</a> : null}
      {message.kind === "location_request" ? <div className="wa-action"><MapPin size={16}/>Location requested · use the preview answer</div> : null}
      {message.kind === "flow" ? <button className="wa-action" type="button" disabled={!onOpenFlow} onClick={onOpenFlow}>{message.flowLabel || "Open Flow"}</button> : null}
    </div>
    {replies ? <div className="wa-replies">{message.options.map(option => <button type="button" key={option.id} disabled={!canReply} onClick={() => onReply?.(option.id, option.label)}>{option.label}</button>)}</div> : null}
    {listOpen ? <PreviewSheet title="Choose an option" onClose={close}><div className="wa-list"><h3>Options</h3>{message.options.map(option => <button key={option.id} type="button" disabled={!canReply} onClick={() => { setListOpen(false); onReply?.(option.id, option.label); }}><span>{option.label}{option.description && <small>{option.description}</small>}</span><span aria-hidden="true">○</span></button>)}</div></PreviewSheet> : null}
  </div>;
}

export function WhatsAppThread({ lines, busy = false, active = false, onReply, onOpenFlow }: { lines: Line[]; busy?: boolean; active?: boolean; onReply?: Reply; onOpenFlow?: () => void }) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => { if (ref.current) ref.current.scrollTop = ref.current.scrollHeight; }, [lines, busy]);
  return <div className="wa-thread" role="log" aria-live="polite" aria-relevant="additions" aria-label="WhatsApp preview messages" ref={ref}><div className="wa-date">Today</div><p className="wa-notice">Preview only. Messages stay in this workspace.</p>{lines.map((line, index) => <WhatsAppMessage key={index} line={line} grouped={index > 0 && lines[index - 1].from === line.from} active={active && index === lines.length - 1} busy={busy} onReply={onReply} onOpenFlow={onOpenFlow}/>)}{busy && <div className="wa-typing" role="status" aria-label="Business is typing"><i/><i/><i/></div>}</div>;
}
