export type ChannelStatusInput = {
  provider?: string;
  status?: string;
  health_status?: string;
  credential_status?: string;
  capability_states?: Array<{ capability: string; status: string; reason?: string }>;
};
export function channelStatus(channel: ChannelStatusInput, explanation?: string) {
  const status = (channel.status || "").toLowerCase();
  if (["disconnected", "archived", "not_connected"].includes(status)) return { label: "Disconnected", tone: "neutral" as const, reason: explanation || (status === "archived" ? "Archived; conversation history is retained." : "This connection is not processing messages.") };
  const credentialProblem = ["expired", "revoked", "requires_reauthorization", "expiring"].includes(channel.credential_status || "");
  const messagingExpected = channel.provider !== "tiktok" && channel.provider !== "test" && channel.provider !== "local_test";
  const capabilityProblem = messagingExpected && ["active", "connected", "healthy", "degraded", "requires_attention"].includes(status) ? channel.capability_states?.find(item =>
    ["inbound_text", "outbound_text"].includes(item.capability) &&
    ["missing_permission", "requires_reauthorization", "failed", "setup_required", "restricted", "unsupported_by_provider", "unverified", "awaiting_permission_review"].includes(item.status)
  ) : undefined;
  if (["degraded", "requires_attention", "failed"].includes(status) || ["degraded", "failed"].includes(channel.health_status || "") || credentialProblem || capabilityProblem) {
    return { label: "Needs attention", tone: "warning" as const, reason: explanation || (credentialProblem ? "Review or reconnect your credentials in Settings." : capabilityProblem ? capabilityProblem.reason || `${capabilityLabel(capabilityProblem.capability)} is not ready. Review permissions and setup in Health.` : "Open Health to review the latest checks and refresh this connection.") };
  }
  if (["active", "connected", "healthy"].includes(status)) return { label: "Connected", tone: "success" as const, reason: explanation || "This connection is enabled. Review Health for delivery readiness." };
  return { label: "Setup incomplete", tone: "neutral" as const, reason: explanation || "Finish authorization, webhook verification and a message test." };
}
export function providerAvailabilityState(loading: boolean, failed: boolean, providerPresent: boolean) {
  if (loading) return "Checking availability…";
  if (failed) return "Availability could not be checked. Try again.";
  if (!providerPresent) return "Not available in this workspace yet";
  return "Provider authorization required; availability depends on workspace configuration";
}
export function capabilityLabel(value: string) {
  const labels: Record<string, string> = { inbound_text: "Receive text", outbound_text: "Send text", inbound_media: "Receive media", outbound_media: "Send media", interactive_messages: "Send buttons and lists", templates: "Send approved templates", whatsapp_flows: "WhatsApp Flows", flows: "WhatsApp Flows", human_handoff: "Hand over to a person", oauth_onboarding: "Connect with provider sign-in", read_receipts: "Read receipts", delivery_receipts: "Delivery updates", commerce_actions: "Commerce actions", message_reactions: "Message reactions", profile_read: "Read profile", content_publishing: "Publish content", comments: "Comments" };
  return labels[value] || value.replace(/_/g, " ");
}
export function isTestConnection(channel: { provider: string }) { return ["test", "local_test"].includes(channel.provider); }
