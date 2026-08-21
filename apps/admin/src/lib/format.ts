export type Row = Record<string, unknown>;

export function isUUID(value: unknown) {
  return typeof value === "string" && /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(value);
}

export function money(minor: unknown, currency = "NGN") {
  const amount = Number(minor ?? 0) / 100;
  try {
    return new Intl.NumberFormat("en-NG", { style: "currency", currency, maximumFractionDigits: 0 }).format(amount);
  } catch {
    return `${currency} ${amount.toLocaleString()}`;
  }
}

export function nairaToMinor(value: string) {
  const cleaned = value.replace(/,/g, "").trim();
  if (!cleaned) return 0;
  return Math.round(Number(cleaned) * 100);
}

export function minorToNairaInput(minor: unknown) {
  return String((Number(minor ?? 0) / 100) || "");
}

export function humanStatus(value: unknown) {
  const raw = String(value ?? "").replace(/_/g, " ");
  if (!raw) return "—";
  return raw.replace(/\b\w/g, (letter: string) => letter.toUpperCase());
}

export function relativeTime(value: unknown) {
  const date = new Date(String(value ?? ""));
  if (Number.isNaN(date.getTime())) return "";
  return date.toLocaleString("en-NG", { hour: "2-digit", minute: "2-digit", day: "numeric", month: "short" });
}

export function isToday(value: unknown) {
  const date = new Date(String(value ?? ""));
  if (Number.isNaN(date.getTime())) return false;
  const now = new Date();
  return date.getFullYear() === now.getFullYear() && date.getMonth() === now.getMonth() && date.getDate() === now.getDate();
}

export function slugify(value: string) {
  return value
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-|-$/g, "");
}

export function parseJSON<T>(value: string | undefined, fallback: T): T {
  try {
    return JSON.parse(value || "{}") as T;
  } catch {
    return fallback;
  }
}

export function nextOrderAction(status: string) {
  switch (status) {
    case "paid":
      return { status: "processing", label: "Start preparing" };
    case "processing":
      return { status: "ready", label: "Mark ready" };
    case "ready":
      return { status: "out_for_delivery", label: "Hand to rider" };
    case "out_for_delivery":
      return { status: "completed", label: "Mark delivered" };
    default:
      return null;
  }
}

export function orderTimeline(status: string) {
  const steps = [
    { key: "placed", label: "Order placed" },
    { key: "paid", label: "Payment received" },
    { key: "processing", label: "Preparing" },
    { key: "ready", label: "Ready" },
    { key: "out_for_delivery", label: "Out for delivery" },
    { key: "completed", label: "Delivered" },
  ];
  const order = ["awaiting_payment", "paid", "processing", "ready", "out_for_delivery", "completed"];
  const current = Math.max(0, order.indexOf(status));
  return steps.map((step, index) => ({ ...step, done: status === "cancelled" ? false : index <= current, current: order[index] === status }));
}

export function setupHref(key: string) {
  switch (key) {
    case "stores":
      return "/stores";
    case "catalogue":
      return "/catalogue";
    case "inventory":
      return "/inventory";
    case "whatsapp":
      return "/settings/whatsapp";
    case "payments":
      return "/settings/payments";
    case "bot":
      return "/assistant";
    case "faqs":
      return "/knowledge";
    case "support":
      return "/conversations";
    default:
      return "/setup";
  }
}

export const moduleHelp: Record<string, { title: string; summary: string }> = {
  WELCOME: { title: "Welcome", summary: "Greets the customer and shows the main menu." },
  ORDER: { title: "Take orders", summary: "Let customers browse your products and place orders." },
  TRACK_ORDER: { title: "Track orders", summary: "Let customers check the status of an existing order." },
  FAQ: { title: "Answer questions", summary: "Answer common questions automatically from your knowledge." },
  COMPLAINT: { title: "Handle complaints", summary: "Allow customers to report a problem and request support." },
  CONTACT_SUPPORT: { title: "Share support options", summary: "Tell customers how to reach your team." },
  HUMAN_HANDOFF: { title: "Talk to a person", summary: "Send customers to a member of your team." },
};

export function roleLabel(role: string) {
  switch (role) {
    case "merchant_admin":
      return "Owner";
    case "store_manager":
      return "Manager";
    case "store_staff":
      return "Staff";
    case "support_agent":
      return "Support";
    case "platform_admin":
      return "Platform";
    case "viewer":
      return "Viewer";
    default:
      return humanStatus(role);
  }
}

export function fulfilmentLabel(mode: string) {
  switch (mode) {
    case "pickup":
      return "Pickup";
    case "customer_rider":
      return "Customer’s rider";
    case "merchant_rider":
      return "Store delivery";
    default:
      return humanStatus(mode);
  }
}
