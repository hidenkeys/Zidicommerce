import { useEffect, useState } from "react";
import { Search, Plus } from "lucide-react";
import { apiGet } from "../../api/client";
import { Badge, Button, EmptyState, LoadingState } from "../ui";
import { experienceMode, experienceModes, experienceStatus } from "../../lib/experience-modes";
import "./flow-studio.css";

type Row = { id: string; name: string; description: string; status: string; updated_at?: string };
type Connection = { id: string; display_name: string };
type Detail = { definition: { mode: string; connection_id?: string; purpose: string }; binding?: { paused: boolean; mode: string; connection_id?: string } };
export function ExperienceLibrary({ rows, connections, onOpen, onCreate, loading = false }: { rows: Row[]; connections: Connection[]; onOpen: (id: string) => void; onCreate?: () => void; loading?: boolean }) {
  const [details, setDetails] = useState<Record<string, Detail>>({});
  const [pending, setPending] = useState(true);
  const [search, setSearch] = useState(""), [mode, setMode] = useState(""), [status, setStatus] = useState(""), [archived, setArchived] = useState(false);
  useEffect(() => {
    let current = true;
    setDetails({}); setPending(true);
    void Promise.allSettled(rows.map(async row => [row.id, (await apiGet<Detail>(`/journeys/${encodeURIComponent(row.id)}`)).data] as const)).then(results => {
      if (!current) return;
      setDetails(Object.fromEntries(results.flatMap(result => result.status === "fulfilled" ? [result.value] : []))); setPending(false);
    });
    return () => { current = false; };
  }, [rows]);
  const label = (row: Row) => row.status === "archived" ? "Archived" : details[row.id] ? experienceStatus(row, details[row.id].binding) : pending ? "Loading…" : "Unavailable";
  const filtered = rows.filter(row => (archived || row.status !== "archived") && (!search || `${row.name} ${row.description}`.toLowerCase().includes(search.trim().toLowerCase())) && (!mode || (details[row.id]?.definition.mode === mode)) && (!status || label(row) === status));
  if (loading) return <LoadingState label="Loading experiences"/>;
  return <div className="flow-library">
    <div className="flow-library-search"><label><Search size={16}/><input type="search" aria-label="Search experiences" placeholder="Search experiences" value={search} onChange={event => setSearch(event.target.value)}/></label><label className="flow-archive-toggle"><input type="checkbox" checked={archived} onChange={event => { setArchived(event.target.checked); if (!event.target.checked && status === "Archived") setStatus(""); }}/>Show archived</label></div>
    <div className="flow-filter-group" role="group" aria-label="Filter by mode"><button aria-pressed={!mode} onClick={() => setMode("")}>All modes</button>{Object.values(experienceModes).map(item => <button key={item.id} aria-pressed={mode === item.id} onClick={() => setMode(mode === item.id ? "" : item.id)}>{item.label}</button>)}</div>
    <div className="flow-filter-group" role="group" aria-label="Filter by status">{["Live", "Draft", "Paused", ...(archived ? ["Archived"] : [])].map(item => <button key={item} aria-pressed={status === item} onClick={() => setStatus(status === item ? "" : item)}>{item}</button>)}</div>
    {pending && <p className="muted" role="status">Loading mode and number details…</p>}
    {filtered.length ? <div className="flow-library-table" role="table" aria-label="Experiences"><div className="flow-library-columns" role="row">{["Name and purpose", "Mode", "Channel / number", "Status", "Last edited", ""].map((heading, index) => <span role="columnheader" key={index}>{heading}</span>)}</div>{filtered.map(row => {
      const detail = details[row.id], metadata = detail ? experienceMode(detail.definition.mode) : undefined;
      const Icon = metadata?.icon;
      const connectionID = detail?.binding ? detail.binding.connection_id : detail?.definition.connection_id;
      return <div className="flow-library-row" role="row" key={row.id}><div role="cell"><strong>{row.name}</strong><small>{detail?.definition.purpose || row.description}</small></div><div role="cell">{metadata && Icon ? <span className="flow-mode-label"><Icon size={16} style={{ color: metadata.accent }}/>{metadata.label}</span> : pending ? "Loading…" : "Unavailable"}</div><div role="cell">{detail ? connectionID ? `${detail.binding ? "" : "Not activated · "}${connections.find(connection => connection.id === connectionID)?.display_name || "Connected number"}` : "Not bound" : "—"}</div><div role="cell"><Badge tone={label(row) === "Live" ? "success" : label(row) === "Paused" ? "warning" : "neutral"}>{label(row)}</Badge></div><div role="cell">{row.updated_at && !Number.isNaN(Date.parse(row.updated_at)) ? new Date(row.updated_at).toLocaleDateString() : "—"}</div><div role="cell"><Button className="secondary" onClick={() => onOpen(row.id)}>Open</Button></div></div>;
    })}</div> : <EmptyState title={rows.length ? "No matching experiences" : "Create your first experience"} body={rows.length ? "Try another search, mode or status, or show archived experiences." : "Choose how customers will interact with your business on WhatsApp."} action={onCreate ? <Button icon={Plus} onClick={onCreate}>{rows.length ? "New experience" : "Create your first experience"}</Button> : undefined}/>}
  </div>;
}
