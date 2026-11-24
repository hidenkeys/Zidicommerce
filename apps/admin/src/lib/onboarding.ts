export type TourStep = { id: string; path: string; title: string; body: string };
export const tourSteps: TourStep[] = [
  { id: "overview", path: "/", title: "Your daily overview", body: "See what needs attention, from orders waiting for fulfilment to conversations that need a person." },
  { id: "catalogue", path: "/catalogue", title: "Build your catalogue", body: "Add products and prices so customers can discover what you sell." },
  { id: "payments", path: "/settings/payments", title: "Prepare payments", body: "Configure how your business collects and verifies customer payments." },
  { id: "channels", path: "/settings/whatsapp", title: "Connect WhatsApp", body: "Connect your business number in Channels and check that customer messaging is ready." },
  { id: "flows", path: "/journeys", title: "Choose an experience", body: "Flow Studio offers Automated Commerce, Guided Questions, WhatsApp Flow and AI Conversation. Choose the mode that suits your customer journey." },
  { id: "conversations", path: "/conversations", title: "Keep human support close", body: "Review customer conversations and take over when a customer needs your team." },
  { id: "team", path: "/team", title: "Bring your team", body: "Review team access and roles so the right people can handle daily work." },
  { id: "setup", path: "/setup", title: "Follow your setup checklist", body: "Finish with the readiness checklist. It links directly to the remaining steps for your store." },
];
export function availableTourSteps(paths: readonly string[]) { return tourSteps.filter(step => paths.includes(step.path)); }
export function tourStorageKey(userId: string, orgId: string) { return `zidi.tour.v1.${userId}.${orgId}`; }
export type TourFlag = "started" | "completed" | "dismissed";
type StorageAccess = Pick<Storage, "getItem" | "setItem">;
export function shouldStartTour(storage: StorageAccess, key: string) {
  try { return !["started", "completed", "dismissed"].includes(storage.getItem(key) || ""); } catch { return true; }
}
export function persistTour(storage: StorageAccess, key: string, flag: TourFlag) {
  try { storage.setItem(key, flag); } catch { /* Storage may be disabled. Keep the current session usable. */ }
}
