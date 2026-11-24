import { Bot, GitBranch, ListChecks, ShoppingBag, MessageCircle } from "lucide-react";

/** Backend identifiers stay unchanged in requests and persisted definitions. */
export const experienceModes = {
  automated_commerce: { id: "automated_commerce", label: "Automated Commerce", description: "Guide shoppers from your catalogue through checkout and support.", bestFor: "Stores, product discovery and orders", icon: ShoppingBag, accent: "var(--z-success)" },
  guided_questions: { id: "guided_questions", label: "Guided Questions", description: "Ask clear questions and route each answer to the right next step.", bestFor: "Intake, qualification and structured conversations", icon: ListChecks, accent: "var(--z-info)" },
  whatsapp_flow: { id: "whatsapp_flow", label: "WhatsApp Flow", description: "Collect information in native WhatsApp screens and forms.", bestFor: "Forms, bookings and multi-screen requests", icon: GitBranch, accent: "var(--z-focus)" },
  ai: { id: "ai", label: "AI Conversation", description: "Answer customer questions with AI and hand over when needed.", bestFor: "Open-ended questions and business knowledge", icon: Bot, accent: "var(--z-warning)" },
  guided_chat: { id: "guided_chat", label: "Legacy guided chat", description: "Maintain an existing guided conversation.", bestFor: "Existing guided chat experiences", icon: MessageCircle, accent: "var(--z-muted)" },
} as const;
export type ExperienceModeId = keyof typeof experienceModes;
export const creationModes = [experienceModes.automated_commerce, experienceModes.guided_questions, experienceModes.whatsapp_flow, experienceModes.ai];
export function experienceMode(mode?: string) {
  return experienceModes[mode as ExperienceModeId];
}
export function experienceModeLabel(mode?: string) {
  return experienceMode(mode)?.label ?? (mode ? mode.replace(/_/g, " ") : "Experience");
}
export type ExperienceStatus = "Live" | "Draft" | "Paused" | "Archived";
/** A published version alone is not live: new sessions require an active binding. */
export function experienceStatus(journey: { status?: string; published_version_id?: string }, binding?: { paused: boolean }): ExperienceStatus {
  if (journey.status === "archived") return "Archived";
  if (binding?.paused || journey.status === "paused") return "Paused";
  if (binding && !binding.paused) return "Live";
  return "Draft";
}
export const previewScenarios = [
  { value: "normal", label: "Happy path" },
  { value: "empty_catalogue", label: "Empty catalogue" },
  { value: "unavailable_stock", label: "Unavailable stock" },
  { value: "payment_failure", label: "Payment failure" },
  { value: "handoff", label: "Handoff" },
  { value: "unsupported_channel", label: "Unsupported channel" },
] as const;
export function scenarioLabel(scenario: string) {
  return previewScenarios.find(item => item.value === scenario)?.label ?? scenario.replace(/_/g, " ");
}
