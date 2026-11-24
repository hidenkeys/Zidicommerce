import type { Detail } from "./types";

/** Provider publication freezes edits, but the journey still needs to be published. */
export function journeyCapabilities(detail: Detail | undefined, manage: boolean, publish: boolean) {
  const archived = detail?.journey.status === "archived";
  const draft = Boolean(detail && ["draft", "validated"].includes(detail.version.status));
  return {
    edit: manage && !archived && draft && !["published", "deprecated"].includes(detail?.deployment?.status ?? ""),
    publish: publish && !archived && draft,
    activate: publish && !archived && Boolean(detail?.version.published_at),
    toggleSessions: publish && !archived && Boolean(detail?.binding),
    clone: manage && !archived && Boolean(detail),
  };
}
