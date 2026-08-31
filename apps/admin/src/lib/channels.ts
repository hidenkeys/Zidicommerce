const usableChannelStatuses = new Set(["active", "connected", "healthy", "degraded", "requires_attention"]);

export function isUsableChannelStatus(status: string | undefined) {
  return usableChannelStatuses.has(String(status ?? "").toLowerCase());
}
