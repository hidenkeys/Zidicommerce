/** Presentation adapters for runtime.OutboundMessage (internal/runtime/dto.go).
 * Keep the runtime payload intact at call sites: selecting an option still submits its ID.
 */
export type WhatsAppOption = { id: string; label: string; description?: string };
export type FlowComponent = { type: string; children?: FlowComponent[]; [key: string]: unknown };
export type FlowScreen = { id: string; title: string; layout: { children: FlowComponent[] }; [key: string]: unknown };
export type FlowDocument = { screens: FlowScreen[] };
export type WhatsAppPreviewMessage = {
  kind: "text" | "buttons" | "list" | "image" | "location_request" | "flow" | "notice" | "fallback";
  text: string; options: WhatsAppOption[]; help?: string; mediaUrl?: string; url?: string;
  flowLabel?: string; direction: "incoming" | "outgoing" | "system"; timestamp?: string;
};
const record = (value: unknown): Record<string, unknown> => value !== null && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
const string = (value: unknown): string => typeof value === "string" ? value : "";
/** Only allow links with an ordinary HTTP(S) scheme, including untrusted preview payloads. */
export function previewURL(value: unknown): string | undefined {
  const candidate = string(value).trim();
  try { const url = new URL(candidate); return ["https:", "http:"].includes(url.protocol) ? url.href : undefined; } catch { return undefined; }
}
export function normalizeMessage(input: unknown): WhatsAppPreviewMessage {
  const raw = record(input), metadata = record(raw.metadata);
  const type = string(raw.type);
  const text = string(raw.text);
  const options: WhatsAppOption[] = Array.isArray(raw.options) ? raw.options.flatMap(value => {
    const option = record(value), id = string(option.id), label = string(option.label);
    return id && label ? [{ id, label, ...(string(option.description) ? { description: string(option.description) } : {}) }] : [];
  }) : [];
  const kind = raw.direction === "internal" || raw.direction === "system" || type === "notice" ? "notice"
    : type === "text" || type === "buttons" || type === "list" || type === "image" || type === "location_request" || type === "flow" ? type
    : raw.location_request === true ? "location_request" : "fallback";
  const direction = kind === "notice" ? "system" : raw.direction === "outgoing" || raw.direction === "outbound" || raw.from === "customer" ? "outgoing" : "incoming";
  const urlInText = text.match(/https?:\/\/[^\s<>]+/)?.[0]?.replace(/[),.;!?]+$/, "");
  return { kind, text: text || (kind === "fallback" ? "This message cannot be displayed in preview." : ""), options,
    direction, help: string(metadata.help) || undefined, mediaUrl: previewURL(raw.media_url),
    url: previewURL(urlInText), flowLabel: string(metadata.flow_cta) || undefined,
    timestamp: string(raw.timestamp) || string(raw.created_at) || undefined };
}
function components(input: unknown, depth = 0): FlowComponent[] {
  if (!Array.isArray(input) || depth > 20) return [];
  return input.flatMap(value => {
    const component = record(value);
    if (!string(component.type)) return [];
    return [{ ...component, type: string(component.type), ...(Array.isArray(component.children) ? { children: components(component.children, depth + 1) } : {}) }];
  });
}
/** Accept compiled JSON objects or their serialized representation, never throw on malformed input. */
export function parseFlowJSON(input: unknown): FlowDocument | undefined {
  let value = input;
  if (typeof input === "string") { try { value = JSON.parse(input); } catch { return undefined; } }
  const document = record(value);
  if (!Array.isArray(document.screens)) return undefined;
  const screens: FlowScreen[] = document.screens.flatMap(value => {
    const screen = record(value), layout = record(screen.layout);
    if (!string(screen.id) || !Array.isArray(layout.children)) return [];
    return [{ ...screen, id: string(screen.id), title: string(screen.title) || string(screen.id), layout: { ...layout, children: components(layout.children) } }];
  });
  return screens.length ? { screens } : undefined;
}
export function parseFlowScreens(input: unknown): FlowScreen[] { return parseFlowJSON(input)?.screens ?? []; }
