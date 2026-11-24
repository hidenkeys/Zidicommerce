import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { apiGet, apiPost } from "../../api/client";
import { useAuth } from "../../auth";
import { hasPermission } from "../../lib/permissions";
import { experienceModes, experienceModeLabel as modeLabel } from "../../lib/experience-modes";
import { Badge, Button, ConfirmButton, EmptyState, Flash, LoadingState, SectionHeader } from "../ui";

type Journey = { id: string; name: string; published_version_id?: string; status: string };
type Version = { id: string; version_number: number; status: string; published_at?: string };
export type ChannelJourney = {
  journey: Journey;
  definition: { connection_id?: string; mode: string };
  versions: Version[];
  binding?: { connection_id?: string; version_id: string; mode: string; paused: boolean; trigger: string };
  channel_default?: { journey_id: string; version_id: string; mode: string };
  publishedDefinition?: { connection_id?: string; mode: string };
};
// The active binding can differ from the latest draft's connection. Read both.
export async function loadChannelJourneys() {
  const rows = (await apiGet<Journey[]>("/journeys")).data;
  return Promise.all(rows.filter(row => row.status !== "archived").map(async row => {
    const detail = (await apiGet<ChannelJourney>(`/journeys/${row.id}`)).data;
    if (row.published_version_id) {
      const published = (await apiGet<ChannelJourney>(`/journeys/${row.id}?version=${encodeURIComponent(row.published_version_id)}`)).data;
      detail.publishedDefinition = published.definition;
    }
    return detail;
  }));
}
export function journeysForConnection(rows: ChannelJourney[], connectionId: string) {
  return rows.filter(row => row.binding?.connection_id === connectionId || row.definition.connection_id === connectionId);
}
export function channelExperienceLabel(rows: ChannelJourney[], connectionId: string) {
  const bound = journeysForConnection(rows, connectionId).filter(row => row.binding?.connection_id === connectionId && !row.binding.paused);
  const defaultId = bound.find(row => row.channel_default)?.channel_default?.journey_id;
  const active = bound.find(row => row.journey.id === defaultId) || bound[0];
  return active ? `${active.journey.name} · ${modeLabel(active.binding!.mode)}${bound.length > 1 ? ` +${bound.length - 1}` : ""}` : "No active experience";
}

export function ChannelExperience({ connectionId }: { connectionId: string }) {
  const { user } = useAuth();
  const canView = hasPermission(user.role, "bot.view");
  const canPublish = hasPermission(user.role, "bot.publish");
  const [rows, setRows] = useState<ChannelJourney[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [reload, setReload] = useState(0);
  const [modes, setModes] = useState<Record<string, string>>({});
  useEffect(() => {
    let current = true;
    setError(""); setLoading(true); setRows([]); setModes({});
    if (!canView) { setLoading(false); return; }
    loadChannelJourneys().then(data => { if (current) setRows(journeysForConnection(data, connectionId)); })
      .catch(reason => { if (current) setError(reason instanceof Error ? reason.message : "Experiences could not be loaded."); })
      .finally(() => { if (current) setLoading(false); });
    return () => { current = false; };
  }, [connectionId, canView, user.organization_id, reload]);
  async function run(action: () => Promise<unknown>) {
    if (!canPublish || busy) return;
    setBusy(true); setError(""); setNotice("");
    try { await action(); setNotice("Experience updated. Existing conversations keep their current version."); setReload(value => value + 1); }
    catch (reason) { setError(reason instanceof Error ? reason.message : "The experience could not be updated."); }
    finally { setBusy(false); }
  }
  if (!canView) return <EmptyState title="Experience access required" body="Your role cannot view Flow Studio journeys." />;
  return <section aria-label="Channel experience">
    <SectionHeader title="Experience" description="Published journeys bound to this connection control new customer conversations." />
    {error ? <><Flash message={error} /><Button onClick={() => setReload(value => value + 1)}>Retry experiences</Button></> : null}
    {notice ? <p role="status">{notice}</p> : null}
    {loading ? <LoadingState label="Loading experiences" /> : !error && !rows.length ? <EmptyState title="No experience assigned" body="Open Flow Studio, choose this connection in journey settings, then publish and activate the journey." action={<Link className="button" to="/journeys">Open Flow Studio</Link>} /> : rows.map(row => {
      const binding = row.binding?.connection_id === connectionId ? row.binding : undefined;
      const published = row.versions.find(version => version.id === row.journey.published_version_id && version.published_at);
      const isDefault = Boolean(binding && !binding.paused && row.channel_default?.journey_id === row.journey.id && row.channel_default.version_id === binding.version_id);
      // Activation takes its connection from the selected published version.
      const canActivate = row.publishedDefinition?.connection_id === connectionId && published;
      const selectedMode = modes[row.journey.id] || binding?.mode || row.publishedDefinition?.mode || row.definition.mode;
      return <div className="channel-experience-row" key={row.journey.id}>
        <h3>{row.journey.name}</h3>
        <p>{modeLabel(binding?.mode || row.definition.mode)} · {binding ? binding.paused ? "Paused for new conversations" : "Active" : "Not activated"}{isDefault ? " · Default experience" : ""}</p>
        {binding ? <Badge tone={binding.paused ? "warning" : "success"}>{binding.paused ? "Paused" : "Active"}</Badge> : null}
        {canPublish && canActivate ? <label className="channels-mode-select">Experience mode<select value={selectedMode} disabled={busy} onChange={event => setModes(current => ({ ...current, [row.journey.id]: event.target.value }))}>{Object.values(experienceModes).map(mode => <option key={mode.id} value={mode.id}>{mode.label}</option>)}</select></label> : null}
        <div className="channel-settings-actions">
          <Link className="button" to={`/journeys?journey=${encodeURIComponent(row.journey.id)}`}>Open in Flow Studio</Link>
          {canPublish && !busy && canActivate ? <ConfirmButton label={binding ? "Apply experience" : "Activate published version"} confirm="New conversations will use this published version. Existing conversations retain their current version." onConfirm={() => void run(() => apiPost(`/journeys/${row.journey.id}/activate`, { version_id: published.id, mode: selectedMode, paused: false }))} /> : null}
          {canPublish && !busy && binding && !binding.paused && !isDefault ? <ConfirmButton label="Use as default experience" confirm="New conversations on this channel will start with this journey. Existing conversations retain their current version." onConfirm={() => void run(() => apiPost(`/journeys/${row.journey.id}/default`, {}))} /> : null}
        </div>
      </div>;
    })}
    {!canPublish ? <p className="read-only-note">Your role can view experiences. Publishing permission is required to change them.</p> : null}
  </section>;
}
