import { useEffect, useMemo, useRef, useState } from "react";
import { Activity, Check, CircleAlert, ExternalLink, KeyRound, Link2, RefreshCw, ShieldCheck, Unplug, X } from "lucide-react";
import { apiGet, apiPost } from "../../api/client";
import { Badge, Button, ConfirmButton, EmptyState, LoadingState, SectionHeader } from "../ui";
import { humanStatus, relativeTime } from "../../lib/format";

const OAUTH_CONNECTION_KEY = "zidi_channel_oauth_connection";

export type ChannelCapabilityState = {
  capability: string;
  status: string;
  reason?: string;
  required_scopes: string[];
  granted_scopes: string[];
  missing_scopes: string[];
  verified_at?: string;
};

export type OAuthChannelDetail = {
  connection: {
    id: string;
    provider: string;
    display_name: string;
    status: string;
    credential_status: string;
    last_connected_at?: string;
    capability_states: ChannelCapabilityState[];
  };
  provider_accounts: Array<{ id: string; business_name?: string; display_name?: string; status: string }>;
  identities: Array<{ id: string; identity_type: string; display_name?: string; external_handle?: string; status: string }>;
  credentials: Array<{ id: string; credential_type: string; status: string; expires_at?: string; last_validated_at?: string }>;
  health: { status: string };
  metrics: { inbound_count: number; outbound_count: number };
};

type ProviderEvent = { id: string; event_type: string; normalized_status: string; received_at: string };
type OAuthStart = { session_id: string; authorization_url: string; expires_at: string };
type OAuthSession = { id: string; status: string; requested_scopes: string[]; granted_scopes: string[]; denied_scopes: string[]; expires_at: string };
type OAuthAsset = {
  id: string;
  oauth_session_id: string;
  provider_asset_id: string;
  asset_type: string;
  display_name: string;
  external_handle?: string;
  eligible: boolean;
  ineligible_reason?: string;
  selected: boolean;
};
type OAuthCallback = { session: OAuthSession; assets: OAuthAsset[] };

const providerCopy: Record<string, { title: string; description: string; account: string }> = {
  instagram: {
    title: "Instagram",
    description: "Authorize an eligible professional account and route customer-initiated Direct messages into Zidi.",
    account: "Instagram professional account",
  },
  facebook: {
    title: "Facebook Messenger",
    description: "Authorize a business user, then explicitly select the Facebook Page Zidi may serve.",
    account: "Facebook Page",
  },
  tiktok: {
    title: "TikTok",
    description: "Connect a TikTok account for approved profile capabilities. Customer messaging is not exposed by the current API.",
    account: "TikTok account",
  },
};

function tone(status: string): "neutral" | "success" | "warning" | "danger" | "info" {
  if (["available", "connected", "healthy", "present", "selected"].includes(status)) return "success";
  if (["setup_required", "awaiting_permission_review", "unverified", "connecting", "expiring"].includes(status)) return "warning";
  if (["missing_permission", "failed", "expired", "revoked", "requires_attention", "requires_reauthorization"].includes(status)) return "danger";
  if (["restricted"].includes(status)) return "info";
  return "neutral";
}

function oauthRedirectURI() {
  return `${window.location.origin}/settings/whatsapp`;
}

function capabilityLabel(capability: string) {
  return humanStatus(capability).replace(/^Oauth\b/, "OAuth");
}

function clearOAuthQuery() {
  const url = new URL(window.location.href);
  ["code", "state", "error", "error_description"].forEach((key) => url.searchParams.delete(key));
  window.history.replaceState({}, "", `${url.pathname}${url.search}${url.hash}`);
}

export function oauthConnectionFromSession() {
  try {
    return window.sessionStorage.getItem(OAUTH_CONNECTION_KEY) || "";
  } catch {
    return "";
  }
}

export function OAuthChannelPanel({
  detail,
  events,
  canManage,
  onChanged,
  onMessage,
}: {
  detail: OAuthChannelDetail;
  events: ProviderEvent[];
  canManage: boolean;
  onChanged: () => Promise<void>;
  onMessage: (message: string, success?: boolean) => void;
}) {
  const { connection } = detail;
  const copy = providerCopy[connection.provider] || { title: humanStatus(connection.provider), description: "Connect an approved provider account.", account: "Provider account" };
  const [assets, setAssets] = useState<OAuthAsset[]>([]);
  const [session, setSession] = useState<OAuthSession | null>(null);
  const [busy, setBusy] = useState(false);
  const callbackStarted = useRef(false);

  const activeAccount = detail.provider_accounts.find((account) => account.status === "connected") || detail.provider_accounts[0];
  const activeIdentity = detail.identities.find((identity) => identity.status === "connected") || detail.identities[0];
  const connected = ["connected", "healthy", "degraded", "requires_attention"].includes(connection.status) && Boolean(activeAccount);
  const messagingCapabilities = connection.capability_states.filter((item) => ["inbound_text", "outbound_text", "inbound_media", "outbound_media", "interactive_messages"].includes(item.capability));
  const messagingAvailable = messagingCapabilities.some((item) => item.status === "available");
  const missingScopes = useMemo(() => Array.from(new Set(connection.capability_states.flatMap((item) => item.missing_scopes || []))).sort(), [connection.capability_states]);
  const grantedScopes = useMemo(() => Array.from(new Set(connection.capability_states.flatMap((item) => item.granted_scopes || []))).sort(), [connection.capability_states]);
  const tokenExpiry = detail.credentials.map((credential) => credential.expires_at).filter(Boolean).sort()[0];
  const latestInbound = events.find((event) => event.event_type.includes("message") || event.event_type.includes("inbound"));

  async function loadAssets() {
    try {
      const response = await apiGet<OAuthAsset[]>(`/channel-platform/connections/${connection.id}/oauth/assets`);
      setAssets(response.data);
    } catch {
      setAssets([]);
    }
  }

  useEffect(() => {
    void loadAssets();
  }, [connection.id]);

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const code = params.get("code") || "";
    const state = params.get("state") || "";
    const providerError = params.get("error_description") || params.get("error") || "";
    if ((!code && !providerError) || oauthConnectionFromSession() !== connection.id || callbackStarted.current) return;
    callbackStarted.current = true;
    clearOAuthQuery();
    if (providerError) {
      window.sessionStorage.removeItem(OAUTH_CONNECTION_KEY);
      onMessage("Provider authorization was cancelled or denied. You can safely try again.");
      return;
    }
    if (!state) {
      window.sessionStorage.removeItem(OAUTH_CONNECTION_KEY);
      onMessage("The provider response was incomplete. Start the connection again.");
      return;
    }
    setBusy(true);
    void apiPost<OAuthCallback>(`/channel-platform/connections/${connection.id}/oauth/callback`, { code, state })
      .then(async (response) => {
        setSession(response.data.session);
        setAssets(response.data.assets);
        onMessage("Authorization completed. Select the business asset Zidi may connect.", true);
        await onChanged();
      })
      .catch((error) => onMessage(error instanceof Error ? error.message : "Provider authorization could not be completed."))
      .finally(() => {
        window.sessionStorage.removeItem(OAUTH_CONNECTION_KEY);
        setBusy(false);
      });
  }, [connection.id, onChanged, onMessage]);

  async function startOAuth() {
    setBusy(true);
    try {
      const response = await apiPost<OAuthStart>(`/channel-platform/connections/${connection.id}/oauth/start`, { redirect_uri: oauthRedirectURI() });
      window.sessionStorage.setItem(OAUTH_CONNECTION_KEY, connection.id);
      window.location.assign(response.data.authorization_url);
    } catch (error) {
      onMessage(error instanceof Error ? error.message : "Provider authorization could not be started.");
      setBusy(false);
    }
  }

  async function selectAsset(asset: OAuthAsset) {
    setBusy(true);
    try {
      await apiPost(`/channel-platform/connections/${connection.id}/oauth/assets/${asset.id}/select`, { session_id: session?.id || asset.oauth_session_id });
      setSession(null);
      onMessage(`${asset.display_name || copy.account} is connected.`, true);
      await onChanged();
      await loadAssets();
    } catch (error) {
      onMessage(error instanceof Error ? error.message : "The provider asset could not be connected.");
    } finally {
      setBusy(false);
    }
  }

  async function cancelOAuth() {
    const sessionID = session?.id || assets.find((asset) => !asset.selected)?.oauth_session_id;
    if (!sessionID) return;
    setBusy(true);
    try {
      await apiPost(`/channel-platform/connections/${connection.id}/oauth/cancel`, { session_id: sessionID });
      setSession(null);
      setAssets([]);
      onMessage("Provider setup was cancelled. No account was connected.", true);
      await onChanged();
    } catch (error) {
      onMessage(error instanceof Error ? error.message : "Provider setup could not be cancelled.");
    } finally {
      setBusy(false);
    }
  }

  async function refreshToken() {
    setBusy(true);
    try {
      await apiPost(`/channel-platform/connections/${connection.id}/oauth/refresh`, {});
      onMessage("Provider credentials were refreshed and revalidated.", true);
      await onChanged();
    } catch (error) {
      onMessage(error instanceof Error ? error.message : "Provider credentials require reconnection.");
    } finally {
      setBusy(false);
    }
  }

  async function disconnect() {
    setBusy(true);
    try {
      await apiPost(`/channel-platform/connections/${connection.id}/oauth/disconnect`, {});
      setAssets([]);
      onMessage(`${copy.title} was disconnected. Conversation and audit history remain available.`, true);
      await onChanged();
    } catch (error) {
      onMessage(error instanceof Error ? error.message : "The provider account could not be disconnected.");
    } finally {
      setBusy(false);
    }
  }

  const hasPendingAssets = assets.some((asset) => !asset.selected);
  const steps = [
    { label: "Setup record", complete: true },
    { label: "Provider authorization", complete: detail.credentials.some((credential) => credential.status === "present") },
    { label: `Select ${copy.account}`, complete: Boolean(activeAccount) },
    { label: "Review permissions", complete: grantedScopes.length > 0 && missingScopes.length === 0 },
    { label: connection.provider === "tiktok" ? "Verify profile" : "Verify webhook", complete: connected },
    { label: connection.provider === "tiktok" ? "Messaging not available" : "Receive controlled test", complete: connection.provider === "tiktok" || Boolean(latestInbound) },
    { label: "Finish setup", complete: connected },
  ];

  return (
    <section className="oauth-channel-panel" aria-label={`${copy.title} onboarding`}>
      <SectionHeader
        title={copy.title}
        description={copy.description}
        action={canManage && connected ? <Button icon={RefreshCw} loading={busy} onClick={() => void refreshToken()}>Refresh credentials</Button> : undefined}
      />

      {connection.provider === "tiktok" ? (
        <div className="channel-limit-notice"><CircleAlert size={18} aria-hidden="true" /><div><strong>Messaging unavailable through the current TikTok API</strong><span>Login Kit profile access can be connected. Zidi will not create an inbox or offer send actions without an approved official messaging API.</span></div></div>
      ) : null}

      <ol className="oauth-progress" aria-label={`${copy.title} setup progress`}>
        {steps.map((step, index) => <li key={step.label} className={step.complete ? "complete" : ""}><span>{step.complete ? <Check size={14} aria-hidden="true" /> : index + 1}</span><small>{step.label}</small></li>)}
      </ol>

      {!connected && !hasPendingAssets ? (
        <div className="oauth-connect-row">
          <div><strong>{connection.status === "disconnected" ? `Reconnect ${copy.title}` : `Connect ${copy.title}`}</strong><span>Provider sign-in opens on the official authorization page. Zidi receives credentials only on the API server.</span></div>
          {canManage ? <Button icon={ExternalLink} className="primary" loading={busy} onClick={() => void startOAuth()}>{connection.status === "disconnected" ? "Reconnect" : "Connect provider"}</Button> : null}
        </div>
      ) : null}

      {busy && !connected && !hasPendingAssets ? <LoadingState label="Completing provider authorization" /> : null}

      {hasPendingAssets ? (
        <section className="oauth-assets">
          <SectionHeader title={`Select ${copy.account}`} description="Only the selected asset becomes a Zidi channel connection." />
          <div className="oauth-asset-list">
            {assets.filter((asset) => !asset.selected).map((asset) => (
              <div className="oauth-asset-row" key={asset.id}>
                <span><strong>{asset.display_name || copy.account}</strong><small>{asset.external_handle ? `@${asset.external_handle.replace(/^@/, "")}` : humanStatus(asset.asset_type)}</small></span>
                {asset.eligible ? <Button icon={Link2} loading={busy} disabled={!canManage} onClick={() => void selectAsset(asset)}>Connect</Button> : <Badge tone="danger">Not eligible</Badge>}
                {!asset.eligible && asset.ineligible_reason ? <p>{asset.ineligible_reason}</p> : null}
              </div>
            ))}
          </div>
          {canManage ? <Button icon={X} className="ghost" loading={busy} onClick={() => void cancelOAuth()}>Cancel setup</Button> : null}
        </section>
      ) : null}

      {connected ? (
        <div className="oauth-account-summary">
          <div><span>Connected account</span><strong>{activeAccount?.display_name || activeAccount?.business_name || copy.account}</strong></div>
          <div><span>Provider identity</span><strong>{activeIdentity?.external_handle ? `@${activeIdentity.external_handle.replace(/^@/, "")}` : activeIdentity?.display_name || humanStatus(activeIdentity?.identity_type || "connected")}</strong></div>
          <div><span>Credential health</span><strong>{humanStatus(connection.credential_status)}</strong></div>
          <div><span>Token expiry</span><strong>{tokenExpiry ? relativeTime(tokenExpiry) : "Provider managed"}</strong></div>
          <div><span>Webhook health</span><strong>{connection.provider === "tiktok" ? "Not applicable" : humanStatus(detail.health.status === "not_checked" ? "subscription configured" : detail.health.status)}</strong></div>
          <div><span>Last inbound</span><strong>{latestInbound ? relativeTime(latestInbound.received_at) : connection.provider === "tiktok" ? "Not supported" : "Awaiting test"}</strong></div>
        </div>
      ) : null}

      <section className="oauth-capabilities">
        <SectionHeader title="Permissions and capabilities" description="Actions are enabled only when provider support, approval, scopes, and asset readiness agree." />
        {connection.capability_states.length ? connection.capability_states.map((capability) => (
          <div className="oauth-capability-row" key={capability.capability}>
            <span><strong>{capabilityLabel(capability.capability)}</strong><small>{capability.reason || (capability.status === "available" ? "Ready for this connection" : "No provider detail supplied")}</small></span>
            <Badge tone={tone(capability.status)}>{humanStatus(capability.status)}</Badge>
            {capability.missing_scopes?.length ? <small className="oauth-scope-note">Missing: {capability.missing_scopes.join(", ")}</small> : null}
          </div>
        )) : <EmptyState title="Capability status is not available" body="Refresh this connection after migrations have initialized its provider capability records." />}
      </section>

      {connected && connection.provider !== "tiktok" ? (
        <div className="oauth-test-row">
          <Activity size={18} aria-hidden="true" />
          <div><strong>{messagingAvailable ? "Controlled message test" : "Messaging awaits provider approval"}</strong><span>{messagingAvailable ? "Send a customer-initiated message to this account, then check activity. Zidi will reply only inside the provider policy window." : "The connection can be inspected with app-role test assets, but merchant messaging stays disabled until Meta grants the required access."}</span></div>
          <Button icon={ShieldCheck} onClick={() => void onChanged()}>Check activity</Button>
        </div>
      ) : null}

      {canManage && connected ? (
        <div className="oauth-connection-actions">
          <Button icon={KeyRound} loading={busy} onClick={() => void startOAuth()}>Review permissions</Button>
          <ConfirmButton label="Disconnect" confirm={`Revoke ${copy.title} authorization and stop this connection? Existing conversation and audit history will remain.`} onConfirm={() => void disconnect()} tone="danger" />
        </div>
      ) : null}

      {!canManage ? <p className="read-only-note">Provider onboarding is read-only for your role.</p> : null}
      {session?.denied_scopes.length ? <p className="oauth-denied-note"><CircleAlert size={16} aria-hidden="true" /> Permission not granted: {session.denied_scopes.join(", ")}</p> : null}
    </section>
  );
}
