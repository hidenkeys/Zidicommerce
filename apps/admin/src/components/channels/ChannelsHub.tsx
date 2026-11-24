import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { Camera, MessageCircle, MessageSquareText, Music2, Plus, RefreshCw } from "lucide-react";
import { apiGet } from "../../api/client";
import { useAuth } from "../../auth";
import { Badge, Button, EmptyState, Flash, SectionHeader } from "../ui";
import { channelStatus, isTestConnection, providerAvailabilityState, type ChannelStatusInput } from "../../lib/channel-status";
import { hasPermission } from "../../lib/permissions";
import { relativeTime } from "../../lib/format";
import "./channels.css";

type Connection = ChannelStatusInput & { id: string; provider: string; display_name: string; environment: string; last_health_check_at?: string };
type Detail = { identities: Array<{ identity_type: string; display_name?: string; external_handle?: string }>; health: { last_check?: { error_message?: string } } };
type WhatsApp = { display_phone_number?: string; last_inbound_at?: string; last_outbound_at?: string; next_action?: string };
type JourneyDetail = { journey: { id: string; name: string }; definition: { connection_id?: string }; binding?: { connection_id?: string; paused: boolean; mode: string; version_id: string }; channel_default?: {journey_id: string; version_id: string; mode: string} };
const otherProviders = [{ key: "instagram", title: "Instagram", icon: Camera }, { key: "facebook", title: "Messenger", icon: MessageSquareText }, { key: "tiktok", title: "TikTok", icon: Music2 }];

export function ChannelsHub({ onConnect, onConnectProvider, canManage }: { onConnect: () => void; onConnectProvider?: (provider: string) => void; canManage: boolean }) {
  const { user } = useAuth();
  const [rows,setRows] = useState<Connection[]>([]);
  const [details,setDetails] = useState<Record<string, Detail>>({});
  const [numbers,setNumbers] = useState<Record<string, WhatsApp>>({});
  const [journeys,setJourneys] = useState<JourneyDetail[]>([]);
  const [journeysLoading,setJourneysLoading] = useState(false);
  const [journeysFailed,setJourneysFailed] = useState(false);
  const [catalog,setCatalog] = useState<Array<{provider:string}>>([]);
  const [catalogLoading,setCatalogLoading] = useState(true);
  const [catalogFailed,setCatalogFailed] = useState(false);
  const [loading,setLoading] = useState(true);
  const [error,setError] = useState("");
  const [reload,setReload] = useState(0);
  const [identityErrors,setIdentityErrors] = useState<Record<string, boolean>>({});
  useEffect(() => {
    let alive = true;
    setLoading(true); setCatalogLoading(true); setCatalogFailed(false); setError("");
    setRows([]); setIdentityErrors({}); setDetails({}); setNumbers({}); setJourneys([]); setCatalog([]); setJourneysFailed(false);
    if (!hasPermission(user.role,"channels.view")) { setLoading(false); setCatalogLoading(false); return; }
    void apiGet<Connection[]>("/channel-platform/connections").then(async response => {
      if (!alive) return;
      setRows(response.data); setLoading(false);
      await Promise.allSettled(response.data.map(async row => {
        try {
          const detail = await apiGet<Detail>(`/channel-platform/connections/${row.id}`);
          if(alive) setDetails(previous => ({...previous,[row.id]: detail.data}));
        } catch { if(alive) setIdentityErrors(previous => ({...previous,[row.id]:true})); }
      }));
      await Promise.allSettled(response.data.filter(row => row.provider === "whatsapp").map(async row => {
        try {
          const number = await apiGet<WhatsApp>(`/channel-platform/connections/${row.id}/whatsapp`);
          if(alive) setNumbers(previous => ({...previous,[row.id]:number.data}));
        } catch { if(alive) setIdentityErrors(previous => ({...previous,[row.id]:true})); }
      }));
    }).catch(reason => { if(alive){setError(reason instanceof Error ? reason.message : "Could not load channels.");setLoading(false);} });
    void apiGet<Array<{provider:string}>>("/channel-platform/providers").then(response => {if(alive)setCatalog(response.data);}).catch(() => {if(alive)setCatalogFailed(true);}).finally(() => {if(alive)setCatalogLoading(false);});
    if(hasPermission(user.role,"bot.view")) {
      setJourneysLoading(true);
      void apiGet<Array<{id:string;status:string}>>("/journeys").then(async response => {
        const results = await Promise.all(response.data.filter(journey => journey.status !== "archived").map(async journey => (await apiGet<JourneyDetail>(`/journeys/${journey.id}`)).data));
        if(alive)setJourneys(results);
      }).catch(() => {if(alive)setJourneysFailed(true);}).finally(() => {if(alive)setJourneysLoading(false);});
    } else setJourneysLoading(false);
    return () => {alive=false;};
  },[reload,user.role,user.organization_id]);
  function connectionRow(row: Connection) {
    const number = numbers[row.id];
    const detail = details[row.id];
    const status = channelStatus(row, detail?.health.last_check?.error_message);
    const identity = detail?.identities.find(item => item.identity_type === "phone_number");
    const phone = number?.display_phone_number || identity?.external_handle || identity?.display_name;
    const bound = journeys.filter(item => item.binding?.connection_id === row.id && !item.binding.paused);
    const active = bound.find(item => item.channel_default?.journey_id === item.journey.id) || bound[0];
    const activity = [number?.last_inbound_at,number?.last_outbound_at,row.last_health_check_at].filter((value):value is string => Boolean(value)).sort().slice(-1)[0];
    return <div className="channels-hub-row" key={row.id}>
      <div className="channels-hub-identity"><span className="channels-hub-icon"><MessageCircle size={20} aria-hidden="true" /></span><div><Link to={`/settings/whatsapp/${row.id}`} className="channels-name">{row.display_name}</Link><small>{phone || (isTestConnection(row) ? "Local test connection" : row.provider === "whatsapp" ? identityErrors[row.id] ? "Number could not be checked" : number ? "Number not configured" : "Loading number…" : row.provider)}</small></div></div>
      <div><Badge tone={status.tone}>{status.label}</Badge><small>{status.reason}</small></div>
      <div>{active ? <><Link to={`/journeys?journey=${active.journey.id}`}>{active.journey.name}</Link><small>{active.binding?.mode.replace(/_/g," ")}</small></> : <span className="channels-muted">{hasPermission(user.role,"bot.view") ? journeysLoading ? "Loading experience…" : journeysFailed ? "Experience could not be checked" : "No active experience found" : "Experience access restricted"}</span>}</div>
      <div className="channels-muted">{activity ? relativeTime(activity) : "No activity recorded"}</div>
    </div>;
  }
  const visible = rows.filter(row => row.status !== "archived" && !isTestConnection(row)).sort((a,b) => Number(b.provider === "whatsapp")-Number(a.provider === "whatsapp"));
  const tests = rows.filter(row => row.status !== "archived" && isTestConnection(row));
  if (!hasPermission(user.role,"channels.view")) return <EmptyState title="Channel access required" body="Your role cannot view channel connections."/>;
  return <div className="channels-hub">
    {error ? <><Flash message={error}/><Button icon={RefreshCw} onClick={() => setReload(value => value+1)}>Retry loading channels</Button></> : null}
    {Object.keys(identityErrors).length ? <Button onClick={() => setReload(value => value+1)}>Retry channel details</Button> : null}
    {journeysFailed ? <Button onClick={() => setReload(value => value+1)}>Retry experiences</Button> : null}
    <SectionHeader title="Your channels" description="Numbers and accounts your customers use to reach you." />
    {loading ? <div className="channels-skeleton" role="status" aria-label="Loading channels"><span/><span/><span/></div> : visible.length ? <div className="channels-hub-list"><div className="channels-hub-columns" aria-hidden="true"><span>Channel</span><span>Status</span><span>Active experience</span><span>Last activity / check</span></div>{visible.map(connectionRow)}</div> : !error ? <EmptyState title="Connect your first WhatsApp number" body="Connect your business number, verify delivery, then choose the experience customers will use." action={canManage ? <Button className="primary" icon={Plus} onClick={onConnect}>Connect WhatsApp</Button> : undefined}/> : null}
    {tests.length ? <details className="channels-test-group"><summary>Test connections ({tests.length})</summary><p className="channels-muted">Local tests do not send messages to real customers.</p>{tests.map(connectionRow)}</details> : null}
    {rows.some(row => row.status === "archived") ? <details className="channels-test-group"><summary>Archived connections</summary>{rows.filter(row => row.status === "archived").map(connectionRow)}</details> : null}
    <SectionHeader title="Add another channel" description="Provider access depends on your workspace and the provider’s approvals."/>
    <div className="channels-other-providers">{otherProviders.map(provider => <div key={provider.key}><provider.icon size={20} aria-hidden="true"/><div><strong>{provider.title}</strong><p>{providerAvailabilityState(catalogLoading,catalogFailed,catalog.some(item => item.provider === provider.key))}</p>{canManage && !catalogLoading && !catalogFailed && catalog.some(item => item.provider === provider.key) && onConnectProvider ? <Button onClick={() => onConnectProvider(provider.key)}>Set up {provider.title}</Button> : null}{provider.key === "tiktok" ? <small>Profile access only; customer messaging is not supported.</small> : null}</div></div>)}</div>
    {catalogFailed ? <Button onClick={() => setReload(value => value+1)}>Retry availability</Button> : null}
    {!canManage ? <p className="channels-muted">Your role can view channels. A workspace administrator can connect or change them.</p> : null}
  </div>;
}
