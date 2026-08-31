import { FormEvent, useEffect, useState, type ComponentType } from "react";
import { Activity, Archive, Camera, Check, Clipboard, Globe2, Headphones, KeyRound, Link2, MessageCircle, Pencil, Plus, RefreshCw, Send, ShieldCheck, Unplug, X, type LucideProps } from "lucide-react";
import { API_BASE_URL, apiDelete, apiGet, apiPatch, apiPost } from "../api/client";
import { useAuth } from "../auth";
import { Badge, Button, Card, ConfirmButton, EmptyState, Flash, FormGrid, IconButton, LoadingState, Metric, Modal, Page, SectionHeader } from "../components/ui";
import { humanStatus, maskPhone, relativeTime } from "../lib/format";
import { launchMetaEmbeddedSignup, type MetaSignupLaunch } from "../lib/metaEmbeddedSignup";
import { hasPermission } from "../lib/permissions";

type ChannelConnection = {
  id: string;
  provider: string;
  display_name: string;
  status: string;
  ownership_model: string;
  environment: string;
  capabilities: string[];
  health_status: string;
  credential_status: string;
  identity_count: number;
  last_connected_at?: string;
  last_health_check_at?: string;
};

type ProviderAccount = { id: string; business_name?: string; display_name?: string; status: string };
type ChannelIdentity = { id: string; identity_type: string; display_name?: string; external_handle?: string; status: string };
type Credential = { id: string; credential_type: string; status: string; has_secret_reference: boolean; reference_type?: string; expires_at?: string; last_validated_at?: string };
type HealthCheck = { status: string; checked_at: string; latency_ms: number; error_code?: string; error_message?: string };
type HealthSummary = { status: string; last_check?: HealthCheck; healthy_count: number; degraded_count: number; failed_count: number };
type MetricsSummary = { inbound_count: number; outbound_count: number; failed_outbound_count: number; delivered_count: number; conversations_started: number; conversations_human_handled: number; conversations_ai_handled: number; average_response_ms: number; provider_error_count: number; days: number };
type ProviderEvent = { id: string; event_type: string; normalized_status: string; received_at: string; processing_error?: string };
type ConnectionDetail = { connection: ChannelConnection; provider_accounts: ProviderAccount[]; identities: ChannelIdentity[]; credentials: Credential[]; health: HealthSummary; metrics: MetricsSummary };
type WhatsAppCredential = { credential_type: string; status: string; configured: boolean; resolvable: boolean; reference_type?: string; resolution_problem?: string };
type WhatsAppChecklist = {
  phone_identity_configured: boolean;
  business_account_known: boolean;
  access_token_configured: boolean;
  app_secret_configured: boolean;
  verify_token_configured: boolean;
  webhook_verified: boolean;
  signature_verified: boolean;
  test_message_ready: boolean;
  test_message_sent: boolean;
  inbound_test_received: boolean;
  ready_to_complete: boolean;
  ready_for_inbound: boolean;
  ready_for_outbound: boolean;
};
type WhatsAppOperationalEvent = { type: string; title: string; status: string; guidance?: string; occurred_at: string };
type WhatsAppConfiguration = {
  phone_number_id: string;
  whatsapp_business_account_id: string;
  meta_business_account_id: string;
  display_phone_number: string;
  graph_api_version: string;
  connection_method: string;
  authorization_status: string;
  authorized_at?: string;
  authorization_expires_at?: string;
  last_authorization_error?: string;
  assisted_setup_status: string;
  assisted_setup_note?: string;
  webhook_status: string;
  setup_state: string;
  webhook_callback_url: string;
  signature_status: string;
  next_action: string;
  last_webhook_verified_at?: string;
  last_webhook_verification_attempt_at?: string;
  last_webhook_verification_failed_at?: string;
  last_webhook_verification_error?: string;
  last_signature_verified_at?: string;
  last_signature_rejected_at?: string;
  last_inbound_at?: string;
  last_outbound_at?: string;
  last_provider_failure_at?: string;
  rate_limited_until?: string;
  last_test_message_at?: string;
  last_test_message_error?: string;
  test_recipient_display?: string;
  last_inbound_test_at?: string;
  credential_rotated_at?: string;
  credentials: WhatsAppCredential[];
  checklist: WhatsAppChecklist;
  operational_events: WhatsAppOperationalEvent[];
  legacy_credentials_found: boolean;
  encrypted_storage_enabled: boolean;
  embedded_signup_available: boolean;
};
type EmbeddedSignupCompletion = { connection_status: string; authorization_status: string; business_name?: string; display_phone_number?: string; ownership_model: string; authorized_at?: string; next_action: string };
type WhatsAppHealth = { status: string; setup_state: string; issues: string[] };
type WhatsAppContactState = { id: string; masked_phone: string; consent_status: string; consent_source: string; last_inbound_at?: string; last_outbound_at?: string; service_window_expires_at?: string };
type WhatsAppTemplate = { id: string; name: string; language: string; category: string; status: string; body: string; provider_template_id?: string; variable_schema: string[]; sample_values: Record<string, string>; rejection_reason?: string; updated_at: string };
type WhatsAppPolicyBlock = { id: string; message_type: string; decision: string; reason: string; created_at: string };
type WhatsAppAdvancedMetrics = { inbound_messages: number; outbound_messages: number; freeform_sends: number; template_sends: number; policy_blocked_sends: number; failed_sends: number; delivered: number; read: number; service_window_open_contacts: number; service_window_closed_blocks: number; opted_in_contacts: number; opted_out_contacts: number; ai_handled_conversations: number; human_handled_conversations: number; handoff_rate: number; average_first_response_ms: number; average_provider_delivery_ms: number; provider_errors: number; rate_limits: number; invalid_recipient_errors: number; invalid_credential_errors: number };
type WhatsAppForm = {
  phone_number_id: string;
  whatsapp_business_account_id: string;
  meta_business_account_id: string;
  display_phone_number: string;
  graph_api_version: string;
};
type TemplateForm = { name: string; language: string; category: string; status: string; body: string; provider_template_id: string; variables: string; sample_values: string; rejection_reason: string };

type ProviderDefinition = {
  key: string;
  label: string;
  description: string;
  icon: ComponentType<LucideProps>;
  meta: boolean;
};

const providers: ProviderDefinition[] = [
  { key: "whatsapp", label: "WhatsApp", description: "Customer messaging, commerce conversations, and human support.", icon: MessageCircle, meta: true },
  { key: "instagram", label: "Instagram", description: "Direct-message conversations connected to the same customer workspace.", icon: Camera, meta: true },
  { key: "web", label: "Web chat", description: "A future website widget using the same conversation and assistant runtime.", icon: Globe2, meta: false },
];

const emptyWhatsAppForm: WhatsAppForm = {
  phone_number_id: "",
  whatsapp_business_account_id: "",
  meta_business_account_id: "",
  display_phone_number: "",
  graph_api_version: "v20.0",
};

const emptyTemplateForm: TemplateForm = { name: "", language: "en", category: "utility", status: "draft", body: "", provider_template_id: "", variables: "", sample_values: "", rejection_reason: "" };

const setupSteps = ["setup_started", "credentials_added", "webhook_verified", "phone_identity_verified", "test_message_ready", "test_message_sent", "inbound_test_received", "connected", "healthy"];

function absoluteWebhookURL(value: string) {
  if (/^https?:\/\//i.test(value)) return value;
  try {
    return `${new URL(API_BASE_URL).origin}${value}`;
  } catch {
    return value;
  }
}

function badgeTone(status: string): "neutral" | "success" | "warning" | "danger" | "info" {
  if (["active", "connected", "healthy", "present", "processed", "verified", "delivered", "read", "sent"].includes(status)) return "success";
  if (["degraded", "connecting", "setup_required", "expiring", "pending", "test_message_ready", "test_message_sent", "inbound_test_received"].includes(status)) return "warning";
  if (["failed", "expired", "revoked", "requires_attention", "requires_reauthorization"].includes(status)) return "danger";
  if (["received", "sandbox"].includes(status)) return "info";
  return "neutral";
}

function ReadinessItem({ complete, label }: { complete: boolean; label: string }) {
  return <div className={complete ? "whatsapp-check complete" : "whatsapp-check"}>{complete ? <Check size={16} aria-hidden="true" /> : <X size={16} aria-hidden="true" />}<span>{label}</span></div>;
}

export function ChannelsPage() {
  const { user } = useAuth();
  const canManage = hasPermission(user.role, "channels.manage");
  const [connections, setConnections] = useState<ChannelConnection[]>([]);
  const [selectedID, setSelectedID] = useState("");
  const [detail, setDetail] = useState<ConnectionDetail | null>(null);
  const [events, setEvents] = useState<ProviderEvent[]>([]);
  const [whatsApp, setWhatsApp] = useState<WhatsAppConfiguration | null>(null);
  const [whatsAppForm, setWhatsAppForm] = useState<WhatsAppForm>(emptyWhatsAppForm);
	const [whatsAppTemplates, setWhatsAppTemplates] = useState<WhatsAppTemplate[]>([]);
	const [whatsAppContacts, setWhatsAppContacts] = useState<WhatsAppContactState[]>([]);
	const [whatsAppMetrics, setWhatsAppMetrics] = useState<WhatsAppAdvancedMetrics | null>(null);
	const [policyBlocks, setPolicyBlocks] = useState<WhatsAppPolicyBlock[]>([]);
	const [templateOpen, setTemplateOpen] = useState(false);
	const [editingTemplateID, setEditingTemplateID] = useState("");
	const [templateForm, setTemplateForm] = useState<TemplateForm>(emptyTemplateForm);
  const [credentialMode, setCredentialMode] = useState<"encrypted" | "reference">("reference");
  const [credentialForm, setCredentialForm] = useState({ credential_type: "access_token", secret: "" });
  const [testForm, setTestForm] = useState({ recipient: "", message: "Zidi WhatsApp connection test. Reply to confirm inbound delivery.", template_id: "", template_variables: {} as Record<string, string> });
  const [loading, setLoading] = useState(true);
  const [detailLoading, setDetailLoading] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [createForm, setCreateForm] = useState({ provider: "whatsapp", display_name: "WhatsApp", ownership_model: "merchant_managed", environment: "sandbox" });
  const [editForm, setEditForm] = useState({ display_name: "", ownership_model: "merchant_managed", environment: "sandbox" });
  const [saving, setSaving] = useState(false);
  const [connectingMeta, setConnectingMeta] = useState(false);
  const [message, setMessage] = useState("");
  const [success, setSuccess] = useState("");

  async function loadConnections(preferredID?: string) {
    try {
      const response = await apiGet<ChannelConnection[]>("/channel-platform/connections");
      setConnections(response.data);
      const nextID = preferredID || selectedID || response.data[0]?.id || "";
      setSelectedID(nextID);
      setMessage("");
      return nextID;
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not load channels");
      return "";
    } finally {
      setLoading(false);
    }
  }

  async function loadDetail(connectionID: string) {
    if (!connectionID) {
      setDetail(null);
      setEvents([]);
      setWhatsApp(null);
		setWhatsAppTemplates([]);
		setWhatsAppContacts([]);
		setWhatsAppMetrics(null);
		setPolicyBlocks([]);
      return;
    }
    setDetailLoading(true);
    try {
      const [detailResponse, eventsResponse] = await Promise.all([
        apiGet<ConnectionDetail>(`/channel-platform/connections/${connectionID}`),
        apiGet<ProviderEvent[]>(`/channel-platform/connections/${connectionID}/events?limit=50`),
      ]);
      setDetail(detailResponse.data);
      setEvents(eventsResponse.data);
      setEditForm({ display_name: detailResponse.data.connection.display_name, ownership_model: detailResponse.data.connection.ownership_model, environment: detailResponse.data.connection.environment });
      if (detailResponse.data.connection.provider === "whatsapp") {
		const [whatsAppResponse, templatesResponse, contactsResponse, metricsResponse, blocksResponse] = await Promise.all([
			apiGet<WhatsAppConfiguration>(`/channel-platform/connections/${connectionID}/whatsapp`),
			apiGet<WhatsAppTemplate[]>(`/channel-platform/connections/${connectionID}/whatsapp/templates`),
			apiGet<WhatsAppContactState[]>(`/channel-platform/connections/${connectionID}/whatsapp/contacts?limit=50`),
			apiGet<WhatsAppAdvancedMetrics>(`/channel-platform/connections/${connectionID}/whatsapp/advanced-metrics`),
			apiGet<WhatsAppPolicyBlock[]>(`/channel-platform/connections/${connectionID}/whatsapp/policy-blocks?limit=20`),
		]);
        const configuration = whatsAppResponse.data;
        setWhatsApp(configuration);
		setWhatsAppTemplates(templatesResponse.data);
		setWhatsAppContacts(contactsResponse.data);
		setWhatsAppMetrics(metricsResponse.data);
		setPolicyBlocks(blocksResponse.data);
        setWhatsAppForm({
          phone_number_id: configuration.phone_number_id || "",
          whatsapp_business_account_id: configuration.whatsapp_business_account_id || "",
          meta_business_account_id: configuration.meta_business_account_id || "",
          display_phone_number: configuration.display_phone_number || "",
          graph_api_version: configuration.graph_api_version || "v20.0",
        });
        setCredentialMode(configuration.encrypted_storage_enabled ? "encrypted" : "reference");
      } else {
        setWhatsApp(null);
		setWhatsAppTemplates([]);
		setWhatsAppContacts([]);
		setWhatsAppMetrics(null);
		setPolicyBlocks([]);
      }
      setMessage("");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not load channel details");
    } finally {
      setDetailLoading(false);
    }
  }

  useEffect(() => {
    void loadConnections();
  }, []);

  useEffect(() => {
    void loadDetail(selectedID);
  }, [selectedID]);

  function beginCreate(provider: ProviderDefinition) {
    setCreateForm({ provider: provider.key, display_name: provider.label, ownership_model: "merchant_managed", environment: "sandbox" });
    setCreateOpen(true);
  }

  async function createConnection(event: FormEvent) {
    event.preventDefault();
    setSaving(true);
    try {
      const response = await apiPost<ChannelConnection>("/channel-platform/connections", { ...createForm, status: "setup_required" });
      setCreateOpen(false);
      setSuccess("Channel setup record created. No external account has been connected.");
      const nextID = await loadConnections(response.data.id);
      await loadDetail(nextID);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not create channel setup");
    } finally {
      setSaving(false);
    }
  }

  async function saveConnection(event: FormEvent) {
    event.preventDefault();
    if (!selectedID) return;
    setSaving(true);
    try {
      await apiPatch<ChannelConnection>(`/channel-platform/connections/${selectedID}`, editForm);
      setSuccess("Channel settings updated.");
      await loadConnections(selectedID);
      await loadDetail(selectedID);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not update channel settings");
    } finally {
      setSaving(false);
    }
  }

  async function archiveConnection() {
    if (!selectedID) return;
    try {
      await apiDelete<ChannelConnection>(`/channel-platform/connections/${selectedID}`);
      setSuccess("Channel connection archived. Existing conversation history remains available.");
      setDetail(null);
      setEvents([]);
      setSelectedID("");
      await loadConnections();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not archive channel");
    }
  }

  async function saveWhatsApp(event: FormEvent) {
    event.preventDefault();
    if (!selectedID) return;
    setSaving(true);
    try {
      const payload: Record<string, string> = {
        phone_number_id: whatsAppForm.phone_number_id,
        whatsapp_business_account_id: whatsAppForm.whatsapp_business_account_id,
        meta_business_account_id: whatsAppForm.meta_business_account_id,
        display_phone_number: whatsAppForm.display_phone_number,
        graph_api_version: whatsAppForm.graph_api_version,
      };
      await apiPatch<WhatsAppConfiguration>(`/channel-platform/connections/${selectedID}/whatsapp`, payload);
      setSuccess("WhatsApp account and phone references saved.");
      await loadConnections(selectedID);
      await loadDetail(selectedID);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not save WhatsApp configuration");
    } finally {
      setSaving(false);
    }
  }

  async function rotateWhatsAppCredential(event: FormEvent) {
    event.preventDefault();
    if (!selectedID || !credentialForm.secret.trim()) return;
    setSaving(true);
    try {
      const payload: Record<string, string> = { credential_type: credentialForm.credential_type };
      payload[credentialMode === "encrypted" ? "secret_value" : "secret_reference"] = credentialForm.secret.trim();
      const response = await apiPost<{ rotated: boolean }>(`/channel-platform/connections/${selectedID}/whatsapp/credentials/rotate`, payload);
      setCredentialForm({ ...credentialForm, secret: "" });
      setSuccess(response.data.rotated ? "Credential rotated. The previous value is no longer available." : "Credential added securely.");
      await loadConnections(selectedID);
      await loadDetail(selectedID);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not update the WhatsApp credential");
    } finally {
      setSaving(false);
    }
  }

  async function sendWhatsAppTest(event: FormEvent) {
    event.preventDefault();
    if (!selectedID) return;
    setSaving(true);
    try {
		const payload = testForm.template_id ? { recipient: testForm.recipient, template_id: testForm.template_id, template_variables: testForm.template_variables } : { recipient: testForm.recipient, message: testForm.message };
      const response = await apiPost<{ recipient_display: string }>(`/channel-platform/connections/${selectedID}/whatsapp/test-message`, payload);
      setTestForm({ ...testForm, recipient: "" });
      setSuccess(`Test message accepted for ${response.data.recipient_display}. Reply from that number to validate inbound delivery.`);
      await loadConnections(selectedID);
      await loadDetail(selectedID);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not send the WhatsApp test message");
    } finally {
      setSaving(false);
    }
  }

	function beginCreateTemplate() {
		setEditingTemplateID("");
		setTemplateForm(emptyTemplateForm);
		setTemplateOpen(true);
	}

	function beginEditTemplate(template: WhatsAppTemplate) {
		setEditingTemplateID(template.id);
		setTemplateForm({ name: template.name, language: template.language, category: template.category, status: template.status, body: template.body, provider_template_id: template.provider_template_id || "", variables: template.variable_schema.join(", "), sample_values: template.variable_schema.map((name) => `${name}=${template.sample_values[name] || ""}`).join(", "), rejection_reason: template.rejection_reason || "" });
		setTemplateOpen(true);
	}

	function templatePayload() {
		const variable_schema = templateForm.variables.split(",").map((item) => item.trim()).filter(Boolean);
		const sample_values: Record<string, string> = {};
		templateForm.sample_values.split(",").forEach((pair) => {
			const [name, ...value] = pair.split("=");
			if (name?.trim()) sample_values[name.trim()] = value.join("=").trim();
		});
		return { name: templateForm.name, language: templateForm.language, category: templateForm.category, status: templateForm.status, body: templateForm.body, provider_template_id: templateForm.provider_template_id, variable_schema, sample_values, rejection_reason: templateForm.rejection_reason, header_metadata: {}, footer_metadata: {}, buttons_metadata: [] };
	}

	async function saveTemplate(event: FormEvent) {
		event.preventDefault();
		if (!selectedID) return;
		setSaving(true);
		try {
			const path = `/channel-platform/connections/${selectedID}/whatsapp/templates${editingTemplateID ? `/${editingTemplateID}` : ""}`;
			if (editingTemplateID) await apiPatch<WhatsAppTemplate>(path, templatePayload());
			else await apiPost<WhatsAppTemplate>(path, templatePayload());
			setTemplateOpen(false);
			setSuccess(editingTemplateID ? "WhatsApp template record updated." : "Local WhatsApp template record created.");
			await loadDetail(selectedID);
		} catch (error) {
			setMessage(error instanceof Error ? error.message : "Could not save the WhatsApp template");
		} finally {
			setSaving(false);
		}
	}

	async function archiveTemplate(templateID: string) {
		if (!selectedID) return;
		setSaving(true);
		try {
			await apiDelete<WhatsAppTemplate>(`/channel-platform/connections/${selectedID}/whatsapp/templates/${templateID}`);
			setSuccess("WhatsApp template archived. It can no longer be used for policy-approved sends.");
			await loadDetail(selectedID);
		} catch (error) {
			setMessage(error instanceof Error ? error.message : "Could not archive the WhatsApp template");
		} finally {
			setSaving(false);
		}
	}

	function templatePreview() {
		const payload = templatePayload();
		let body = payload.body;
		payload.variable_schema.forEach((name, index) => {
			const value = payload.sample_values[name] || `{{${name}}}`;
			body = body.split(`{{${name}}}`).join(value).split(`{{${index + 1}}}`).join(value);
		});
		return body;
	}

  async function completeWhatsAppSetup() {
    if (!selectedID) return;
    setSaving(true);
    try {
      await apiPost(`/channel-platform/connections/${selectedID}/whatsapp/complete`, {});
      setSuccess("WhatsApp validation completed. Run the final health check.");
      await loadConnections(selectedID);
      await loadDetail(selectedID);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not complete WhatsApp setup");
    } finally {
      setSaving(false);
    }
  }

  async function migrateLegacyCredentials() {
    if (!selectedID) return;
    setSaving(true);
    try {
      await apiPost(`/channel-platform/connections/${selectedID}/whatsapp/migrate-legacy`, {});
      setSuccess("Legacy WhatsApp settings migrated. The original values were retained for rollback.");
      await loadConnections(selectedID);
      await loadDetail(selectedID);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not migrate legacy WhatsApp settings");
    } finally {
      setSaving(false);
    }
  }

  async function checkWhatsAppHealth() {
    if (!selectedID) return;
    setSaving(true);
    try {
      const response = await apiPost<WhatsAppHealth>(`/channel-platform/connections/${selectedID}/whatsapp/health-check`, {});
      setSuccess(response.data.status === "healthy" && response.data.setup_state === "healthy" ? "WhatsApp is connected and healthy." : response.data.status === "healthy" ? `Transport checks passed. Continue from ${humanStatus(response.data.setup_state)}.` : `WhatsApp requires attention: ${response.data.issues.map(humanStatus).join(", ")}.`);
      await loadConnections(selectedID);
      await loadDetail(selectedID);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not evaluate WhatsApp health");
    } finally {
      setSaving(false);
    }
  }

  async function connectWithMeta() {
    if (!selectedID || !whatsApp?.embedded_signup_available) return;
    setConnectingMeta(true);
    setMessage("");
    try {
      const initiation = await apiPost<MetaSignupLaunch & { attempt_token: string }>(`/channel-platform/connections/${selectedID}/whatsapp/embedded-signup/initiate`, {});
      const result = await launchMetaEmbeddedSignup(initiation.data);
      const completion = await apiPost<EmbeddedSignupCompletion>(`/channel-platform/connections/${selectedID}/whatsapp/embedded-signup/complete`, {
        attempt_token: initiation.data.attempt_token,
        authorization_code: result.authorizationCode,
        whatsapp_business_account_id: result.whatsappBusinessAccountID,
        phone_number_id: result.phoneNumberID,
      });
      setSuccess(`${maskPhone(completion.data.display_phone_number, "WhatsApp")} is authorized. Complete the end-to-end message check before relying on the connection.`);
      await loadConnections(selectedID);
      await loadDetail(selectedID);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not complete Meta signup");
    } finally {
      setConnectingMeta(false);
    }
  }

  async function requestZidiHelp() {
    if (!selectedID) return;
    setSaving(true);
    setMessage("");
    try {
      await apiPost<WhatsAppConfiguration>(`/channel-platform/connections/${selectedID}/whatsapp/assisted-setup`, { status: "requested" });
      setSuccess("Assisted setup requested. Zidi will track the Meta steps and tell you when your action is required.");
      await loadConnections(selectedID);
      await loadDetail(selectedID);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not request assisted setup");
    } finally {
      setSaving(false);
    }
  }

  async function updateAssistedStatus(status: string) {
    if (!selectedID) return;
    setSaving(true);
    try {
      await apiPost<WhatsAppConfiguration>(`/channel-platform/connections/${selectedID}/whatsapp/assisted-setup`, { status, note: status === "awaiting_merchant_action" ? "Complete the Meta authorization step from this page." : "" });
      setSuccess("Assisted setup status updated.");
      await loadConnections(selectedID);
      await loadDetail(selectedID);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not update assisted setup");
    } finally {
      setSaving(false);
    }
  }

  async function setConnectionStatus(status: "disconnected" | "connecting") {
    if (!selectedID) return;
    setSaving(true);
    try {
      await apiPatch<ChannelConnection>(`/channel-platform/connections/${selectedID}`, { status });
      setSuccess(status === "disconnected" ? "WhatsApp disconnected from active processing. Its history remains available." : "Connection reopened. Authorize with Meta again if required.");
      await loadConnections(selectedID);
      await loadDetail(selectedID);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not update connection status");
    } finally {
      setSaving(false);
    }
  }

  async function copyWebhookURL() {
    try {
      await navigator.clipboard.writeText(absoluteWebhookURL(whatsApp?.webhook_callback_url || ""));
      setSuccess("Webhook URL copied.");
    } catch {
      setMessage("Could not copy the webhook URL.");
    }
  }

  const activeConnections = connections.filter((connection) => connection.status !== "archived");
  const selected = detail?.connection;
  const currentSetupStep = whatsApp ? setupSteps.indexOf(whatsApp.setup_state) : -1;
  const webhookURL = whatsApp ? absoluteWebhookURL(whatsApp.webhook_callback_url) : "";

  return (
    <Page
      title="Channels"
      description="Customer touchpoints connected to Zidi's shared conversation and operations platform."
      help="WhatsApp uses Meta's official authorization flow. Zidi manages the connection, messaging runtime, health, and activity after authorization."
      actions={canManage ? <Button icon={Plus} className="primary" onClick={() => beginCreate(providers[0])}>Add setup record</Button> : undefined}
    >
      <Flash message={message} />
      <Flash message={success} tone="success" />

      <SectionHeader title="Channel availability" description="WhatsApp is available now. Instagram and web chat remain disabled." />
      <div className="channel-provider-grid">
        {providers.map((provider) => {
          const Icon = provider.icon;
          const providerConnections = connections.filter((connection) => connection.provider === provider.key && connection.status !== "archived");
          const best = providerConnections.find((connection) => ["active", "connected", "healthy"].includes(connection.status)) || providerConnections[0];
          return (
            <Card key={provider.key} className="channel-provider-card">
              <div className="channel-provider-heading"><span className="channel-provider-icon"><Icon size={20} aria-hidden="true" /></span><Badge tone={best ? badgeTone(best.status) : "neutral"}>{best ? humanStatus(best.status) : "Not connected"}</Badge></div>
              <h3>{provider.label}</h3>
              <p>{provider.description}</p>
              <small>{providerConnections.length ? `${providerConnections.length} setup record${providerConnections.length === 1 ? "" : "s"}` : "No setup record"}</small>
              <div className="channel-provider-actions">
                {provider.key === "whatsapp" ? (best ? <Button icon={KeyRound} onClick={() => setSelectedID(best.id)}>Configure WhatsApp</Button> : canManage ? <Button icon={Plus} onClick={() => beginCreate(provider)}>Prepare WhatsApp</Button> : null) : canManage ? <Button icon={Plus} onClick={() => beginCreate(provider)}>Prepare setup</Button> : null}
                {provider.key !== "whatsapp" ? <button type="button" disabled title="This provider adapter is not enabled">Provider unavailable</button> : null}
              </div>
            </Card>
          );
        })}
      </div>

      <SectionHeader title="Connection records" description="Database-backed setup, ownership, health, and activity for this organization." />
      {loading ? <LoadingState label="Loading channels" /> : connections.length === 0 ? (
        <EmptyState title="No channel setup records" body="Prepare a channel record to choose ownership and track future connection readiness." action={canManage ? <Button icon={Plus} onClick={() => beginCreate(providers[0])}>Prepare a channel</Button> : undefined} />
      ) : (
        <div className="channel-workspace">
          <Card className="channel-connection-list">
            <div className="channel-list-heading"><strong>{activeConnections.length} active setup record{activeConnections.length === 1 ? "" : "s"}</strong><span>{connections.length - activeConnections.length} archived</span></div>
            {connections.map((connection) => (
              <button key={connection.id} type="button" className={`channel-list-item ${selectedID === connection.id ? "selected" : ""}`} onClick={() => setSelectedID(connection.id)}>
                <span><strong>{connection.display_name}</strong><small>{humanStatus(connection.provider)} · {humanStatus(connection.ownership_model)}</small></span>
                <Badge tone={badgeTone(connection.status)}>{humanStatus(connection.status)}</Badge>
              </button>
            ))}
          </Card>

          <div className="channel-detail-stack">
            {detailLoading ? <Card><LoadingState label="Loading channel details" /></Card> : !detail || !selected ? <Card><EmptyState title="Select a connection" body="Review setup ownership, health, metrics, identities, and provider events." /></Card> : <>
              <Card>
                <SectionHeader title={selected.display_name} description={`${humanStatus(selected.provider)} connection foundation`} action={<Button icon={RefreshCw} onClick={() => void loadDetail(selected.id)}>Refresh</Button>} />
                <div className="channel-status-row">
                  <Badge tone={badgeTone(selected.status)}>{humanStatus(selected.status)}</Badge>
                  <Badge tone={badgeTone(selected.health_status)}>{humanStatus(selected.health_status)}</Badge>
                  <Badge>{humanStatus(selected.environment)}</Badge>
                </div>
                <div className="channel-facts">
                  <div><span>Ownership</span><strong>{humanStatus(selected.ownership_model)}</strong></div>
                  <div><span>Credential state</span><strong>{humanStatus(selected.credential_status)}</strong></div>
                  <div><span>Identities</span><strong>{selected.identity_count}</strong></div>
                  <div><span>Last health check</span><strong>{selected.last_health_check_at ? relativeTime(selected.last_health_check_at) : "Not checked"}</strong></div>
                </div>
                <div className="capability-list">{selected.capabilities.length ? selected.capabilities.map((capability) => <Badge key={capability} tone="info">{humanStatus(capability)}</Badge>) : <span className="muted">No provider capabilities declared.</span>}</div>
              </Card>

              {selected.provider === "whatsapp" && whatsApp ? <Card className="whatsapp-setup-panel">
                <SectionHeader title="WhatsApp" description="Connect your business WhatsApp to Zidi conversations, customers, and orders." action={canManage ? <Button icon={Activity} onClick={() => void checkWhatsAppHealth()} loading={saving}>Check health</Button> : undefined} />
                <div className="whatsapp-state-banner">
                  <span><small>Connection</small><Badge tone={badgeTone(selected.status)}>{humanStatus(selected.status)}</Badge></span>
                  <span><small>Authorization</small><Badge tone={badgeTone(whatsApp.authorization_status)}>{humanStatus(whatsApp.authorization_status)}</Badge></span>
                  <strong>{whatsApp.next_action}</strong>
                </div>

                {whatsApp.authorization_status !== "authorized" ? <section className="whatsapp-connect-panel">
                  <div>
                    <h3>Connect WhatsApp</h3>
                    <p>Meta handles sign-in, business selection, phone selection, and authorization. Zidi stores the resulting connection securely.</p>
                  </div>
                  {canManage ? <div className="whatsapp-connect-actions">
                    <Button icon={Link2} className="primary" loading={connectingMeta} disabled={!whatsApp.embedded_signup_available} onClick={() => void connectWithMeta()}>Connect with Meta</Button>
                    <Button icon={Headphones} loading={saving} onClick={() => void requestZidiHelp()}>Let Zidi help</Button>
                  </div> : null}
                  {!whatsApp.embedded_signup_available ? <p className="whatsapp-config-warning">Connect with Meta is unavailable until the Zidi Meta app and secure server credentials are configured.</p> : null}
                </section> : <section className="whatsapp-connected-summary">
                  <div><span>Business</span><strong>{detail.provider_accounts[0]?.business_name || detail.provider_accounts[0]?.display_name || "WhatsApp Business"}</strong></div>
                  <div><span>Phone</span><strong>{maskPhone(whatsApp.display_phone_number, "Connected number")}</strong></div>
                  <div><span>Managed by</span><strong>{selected.ownership_model === "zidi_managed" ? "Zidi" : "You"}</strong></div>
                  <div><span>Authorized</span><strong>{whatsApp.authorized_at ? relativeTime(whatsApp.authorized_at) : "Connected"}</strong></div>
                </section>}

                {whatsApp.assisted_setup_status !== "not_requested" ? <div className="whatsapp-assisted-status">
                  <span><small>Assisted setup</small><Badge tone={badgeTone(whatsApp.assisted_setup_status)}>{humanStatus(whatsApp.assisted_setup_status)}</Badge></span>
                  <p>{whatsApp.assisted_setup_note || (whatsApp.assisted_setup_status === "awaiting_merchant_action" ? "Your action is required in Meta before setup can continue." : "Zidi is tracking the remaining Meta setup steps.")}</p>
                  {user.role === "platform_admin" && !["completed", "connected"].includes(whatsApp.assisted_setup_status) ? <div className="channel-settings-actions">
                    <Button icon={RefreshCw} loading={saving} onClick={() => void updateAssistedStatus("in_progress")}>In progress</Button>
                    <Button icon={Headphones} loading={saving} onClick={() => void updateAssistedStatus("awaiting_merchant_action")}>Awaiting merchant</Button>
                  </div> : null}
                </div> : null}

                <nav className="channel-subnav" aria-label="WhatsApp connection sections">
				  <a href="#whatsapp-connection">Connection</a><a href="#whatsapp-health">Health</a><a href="#whatsapp-messaging">Messaging</a><a href="#whatsapp-policy">Policy</a><a href="#whatsapp-templates">Templates</a><a href="#whatsapp-contacts">Contacts</a><a href="#whatsapp-webhooks">Webhooks</a><a href="#whatsapp-activity">Activity</a><a href="#whatsapp-analytics">Analytics</a><a href="#whatsapp-settings">Settings</a>
                </nav>

                <section className="whatsapp-stage" id="whatsapp-connection">
                  <SectionHeader title="Connection" description="Meta authorization and Zidi readiness are tracked separately" />
                  <ol className="whatsapp-progress" aria-label="WhatsApp onboarding progress">
                    {setupSteps.map((step, index) => <li key={step} className={index <= currentSetupStep ? "complete" : ""}><span>{index < currentSetupStep ? <Check size={14} aria-hidden="true" /> : index + 1}</span><small>{humanStatus(step)}</small></li>)}
                  </ol>
                  <div className="whatsapp-readiness">
                    <div><span>Inbound</span><Badge tone={whatsApp.checklist.ready_for_inbound ? "success" : "warning"}>{whatsApp.checklist.ready_for_inbound ? "Ready" : "Setup required"}</Badge></div>
                    <div><span>Outbound</span><Badge tone={whatsApp.checklist.ready_for_outbound ? "success" : "warning"}>{whatsApp.checklist.ready_for_outbound ? "Ready" : "Setup required"}</Badge></div>
                    <div><span>Webhook</span><Badge tone={badgeTone(whatsApp.webhook_status)}>{humanStatus(whatsApp.webhook_status)}</Badge></div>
                    <div><span>Signed events</span><Badge tone={badgeTone(whatsApp.signature_status)}>{humanStatus(whatsApp.signature_status)}</Badge></div>
                  </div>
                </section>

                {(user.role === "platform_admin" || selected.ownership_model === "zidi_managed" || whatsApp.legacy_credentials_found) ? <section className="whatsapp-stage whatsapp-operator-panel">
                  <SectionHeader title="Operator recovery" description="Manual references are retained for assisted and legacy connections only" />
                  {canManage ? <form className="form-grid whatsapp-config-form" onSubmit={saveWhatsApp}>
                    <label>Phone number ID<input inputMode="numeric" value={whatsAppForm.phone_number_id} onChange={(event) => setWhatsAppForm({ ...whatsAppForm, phone_number_id: event.target.value })} /></label>
                    <label>Display phone number<input value={whatsAppForm.display_phone_number} onChange={(event) => setWhatsAppForm({ ...whatsAppForm, display_phone_number: event.target.value })} /></label>
                    <label>WhatsApp business account ID<input value={whatsAppForm.whatsapp_business_account_id} onChange={(event) => setWhatsAppForm({ ...whatsAppForm, whatsapp_business_account_id: event.target.value })} /></label>
                    <label>Meta business account ID<input value={whatsAppForm.meta_business_account_id} onChange={(event) => setWhatsAppForm({ ...whatsAppForm, meta_business_account_id: event.target.value })} /></label>
                    <label>Graph API version<input value={whatsAppForm.graph_api_version} onChange={(event) => setWhatsAppForm({ ...whatsAppForm, graph_api_version: event.target.value })} /></label>
                    <div className="full channel-settings-actions"><Button type="submit" icon={ShieldCheck} loading={saving}>Save references</Button></div>
                  </form> : null}
                  <div className="whatsapp-credential-status">
                    {whatsApp.credentials.map((credential) => <span key={credential.credential_type}><strong>{humanStatus(credential.credential_type)}</strong><Badge tone={credential.resolvable ? "success" : credential.configured ? "warning" : "neutral"}>{credential.resolvable ? "Available" : credential.configured ? "Unresolved" : "Missing"}</Badge></span>)}
                  </div>
                  {canManage ? <form className="form-grid whatsapp-rotation-form" onSubmit={rotateWhatsAppCredential}>
                    <label>Credential<select value={credentialForm.credential_type} onChange={(event) => setCredentialForm({ ...credentialForm, credential_type: event.target.value, secret: "" })}><option value="access_token">Access token</option><option value="app_secret">App secret</option><option value="verify_token">Verify token</option></select></label>
                    <label>{credentialMode === "encrypted" ? "New credential value" : "New secret reference"}<input type={credentialMode === "encrypted" ? "password" : "text"} autoComplete="new-password" value={credentialForm.secret} onChange={(event) => setCredentialForm({ ...credentialForm, secret: event.target.value })} required /></label>
                    <div className="full whatsapp-credential-heading">
                      {whatsApp.encrypted_storage_enabled ? <div className="credential-mode" role="group" aria-label="Credential input mode"><button type="button" className={credentialMode === "encrypted" ? "active" : ""} onClick={() => { setCredentialMode("encrypted"); setCredentialForm({ ...credentialForm, secret: "" }); }}>Encrypted value</button><button type="button" className={credentialMode === "reference" ? "active" : ""} onClick={() => { setCredentialMode("reference"); setCredentialForm({ ...credentialForm, secret: "" }); }}>Secret reference</button></div> : <Badge tone="info">Secret references only</Badge>}
                      <div className="channel-settings-actions"><Button type="submit" icon={KeyRound} loading={saving}>Save credential</Button>{whatsApp.legacy_credentials_found ? <Button type="button" icon={RefreshCw} loading={saving} onClick={() => void migrateLegacyCredentials()}>Migrate legacy settings</Button> : null}</div>
                    </div>
                  </form> : null}
                </section> : null}

                <section className="whatsapp-stage" id="whatsapp-webhooks">
                  <SectionHeader title="Webhooks" description={whatsApp.connection_method === "embedded_signup" ? "Zidi subscribes the authorized account to its shared Meta app webhook" : "Manual connections retain their connection-specific callback"} />
                  <div className="whatsapp-webhook-row"><span><small>Callback URL</small><code>{webhookURL}</code></span><IconButton label="Copy webhook URL" icon={Clipboard} onClick={() => void copyWebhookURL()} /></div>
                  <div className="whatsapp-checklist"><ReadinessItem complete={whatsApp.checklist.verify_token_configured} label="Verification configured" /><ReadinessItem complete={whatsApp.checklist.webhook_verified} label="WABA subscribed" /><ReadinessItem complete={whatsApp.checklist.signature_verified} label="Signed event observed" /></div>
                  <div className="whatsapp-timestamps"><span><small>Last verification</small><strong>{whatsApp.last_webhook_verification_attempt_at ? relativeTime(whatsApp.last_webhook_verification_attempt_at) : "Not attempted"}</strong></span><span><small>Last signed webhook</small><strong>{whatsApp.last_signature_verified_at ? relativeTime(whatsApp.last_signature_verified_at) : "Not observed"}</strong></span><span><small>Last inbound event</small><strong>{whatsApp.last_inbound_at ? relativeTime(whatsApp.last_inbound_at) : "None"}</strong></span></div>
                </section>

                <section className="whatsapp-stage" id="whatsapp-messaging">
				  <SectionHeader title="Messaging check" description="Free-form tests require an open service window; an approved template is required otherwise" />
                  <div className="whatsapp-checklist">
                    <ReadinessItem complete={whatsApp.checklist.test_message_ready} label="Ready to test" />
                    <ReadinessItem complete={whatsApp.checklist.test_message_sent} label="Outbound accepted" />
                    <ReadinessItem complete={whatsApp.checklist.inbound_test_received} label="Inbound reply received" />
                  </div>
                  {canManage ? <form className="form-grid whatsapp-test-form" onSubmit={sendWhatsAppTest}>
                    <label>Approved test recipient<input inputMode="tel" autoComplete="off" value={testForm.recipient} onChange={(event) => setTestForm({ ...testForm, recipient: event.target.value })} placeholder="+2348000000000" required /></label>
					<label>Send mode<select value={testForm.template_id} onChange={(event) => setTestForm({ ...testForm, template_id: event.target.value, template_variables: {} })}><option value="">Free-form service reply</option>{whatsAppTemplates.filter((template) => template.status === "approved").map((template) => <option key={template.id} value={template.id}>{template.name} · {template.language}</option>)}</select></label>
					{testForm.template_id ? whatsAppTemplates.find((template) => template.id === testForm.template_id)?.variable_schema.map((name) => <label key={name}>{humanStatus(name)}<input value={testForm.template_variables[name] || ""} onChange={(event) => setTestForm({ ...testForm, template_variables: { ...testForm.template_variables, [name]: event.target.value } })} required /></label>) : <label>Test message<input value={testForm.message} onChange={(event) => setTestForm({ ...testForm, message: event.target.value })} maxLength={1000} required /></label>}
                    <div className="full channel-settings-actions"><Button type="submit" icon={Send} className="primary" loading={saving} disabled={!whatsApp.checklist.test_message_ready}>Send test message</Button>{whatsApp.checklist.ready_to_complete ? <Button type="button" icon={ShieldCheck} loading={saving} onClick={() => void completeWhatsAppSetup()}>Complete validation</Button> : null}</div>
                  </form> : null}
                  {whatsApp.test_recipient_display ? <p className="whatsapp-test-result">Latest approved recipient: <strong>{whatsApp.test_recipient_display}</strong> · outbound {whatsApp.last_test_message_at ? relativeTime(whatsApp.last_test_message_at) : "not sent"} · inbound {whatsApp.last_inbound_test_at ? relativeTime(whatsApp.last_inbound_test_at) : "awaiting reply"}</p> : null}
                </section>

				<section className="whatsapp-stage" id="whatsapp-policy">
				  <SectionHeader title="Messaging policy" description="Zidi evaluates consent and the 24-hour customer-service window before every Meta request" />
				  <div className="whatsapp-checklist"><ReadinessItem complete label="Free-form inside open window" /><ReadinessItem complete={whatsAppTemplates.some((template) => template.status === "approved")} label="Approved template available" /><ReadinessItem complete={(whatsAppMetrics?.opted_out_contacts || 0) === 0} label="No opted-out contacts" /></div>
				  <div className="whatsapp-timestamps"><span><small>Open windows</small><strong>{whatsAppMetrics?.service_window_open_contacts || 0}</strong></span><span><small>Opted in</small><strong>{whatsAppMetrics?.opted_in_contacts || 0}</strong></span><span><small>Opted out</small><strong>{whatsAppMetrics?.opted_out_contacts || 0}</strong></span><span><small>Policy blocks</small><strong>{whatsAppMetrics?.policy_blocked_sends || 0}</strong></span></div>
				</section>

				<section className="whatsapp-stage" id="whatsapp-templates">
				  <SectionHeader title="Templates" description="Local mirrors of Meta templates; only records marked approved can send outside the service window" action={canManage ? <Button icon={Plus} onClick={beginCreateTemplate}>Add template</Button> : undefined} />
				  {whatsAppTemplates.length ? <div className="table-wrap"><table><thead><tr><th>Template</th><th>Category</th><th>Status</th><th>Updated</th>{canManage ? <th><span className="sr-only">Actions</span></th> : null}</tr></thead><tbody>{whatsAppTemplates.map((template) => <tr key={template.id}><td><strong>{template.name}</strong><small>{template.language} · {template.variable_schema.length} variables</small></td><td>{humanStatus(template.category)}</td><td><Badge tone={badgeTone(template.status)}>{humanStatus(template.status)}</Badge></td><td>{relativeTime(template.updated_at)}</td>{canManage ? <td><div className="table-actions"><IconButton label={`Edit ${template.name}`} icon={Pencil} onClick={() => beginEditTemplate(template)} />{template.status !== "archived" ? <IconButton label={`Archive ${template.name}`} icon={Archive} onClick={() => void archiveTemplate(template.id)} /> : null}</div></td> : null}</tr>)}</tbody></table></div> : <EmptyState title="No template records" body="Mirror an approved Meta template here before sending outside a customer-service window." />}
				</section>

				<section className="whatsapp-stage" id="whatsapp-contacts">
				  <SectionHeader title="Contact consent and service windows" description="Identifiers remain masked; inbound messages extend the window for 24 hours" />
				  {whatsAppContacts.length ? <div className="table-wrap"><table><thead><tr><th>Contact</th><th>Consent</th><th>Service window</th><th>Last inbound</th></tr></thead><tbody>{whatsAppContacts.map((contact) => { const open = Boolean(contact.service_window_expires_at && new Date(contact.service_window_expires_at) > new Date()); return <tr key={contact.id}><td>{contact.masked_phone}</td><td><Badge tone={contact.consent_status === "opted_out" ? "danger" : contact.consent_status === "opted_in" ? "success" : "neutral"}>{humanStatus(contact.consent_status)}</Badge></td><td><Badge tone={open ? "success" : "warning"}>{open ? "Open" : "Closed"}</Badge>{contact.service_window_expires_at ? <small>{open ? `until ${relativeTime(contact.service_window_expires_at)}` : `expired ${relativeTime(contact.service_window_expires_at)}`}</small> : null}</td><td>{contact.last_inbound_at ? relativeTime(contact.last_inbound_at) : "None"}</td></tr>; })}</tbody></table></div> : <EmptyState title="No WhatsApp contact state" body="Contact consent and service-window state appears after a signed inbound message or approved template send." />}
				</section>

				<section className="whatsapp-stage">
				  <SectionHeader title="Recent policy blocks" description="Messages stopped before Meta was called" />
				  {policyBlocks.length ? <div className="table-wrap"><table><thead><tr><th>Reason</th><th>Message</th><th>When</th></tr></thead><tbody>{policyBlocks.map((block) => <tr key={block.id}><td><Badge tone="warning">{humanStatus(block.decision)}</Badge><small>{block.reason}</small></td><td>{humanStatus(block.message_type)}</td><td>{relativeTime(block.created_at)}</td></tr>)}</tbody></table></div> : <EmptyState title="No policy blocks" body="No WhatsApp message has been blocked by consent, window, or template policy." />}
				</section>

				<section className="whatsapp-stage">
				  <SectionHeader title="Recent provider errors" description="Meta or transport failures after policy allowed a request" />
				  {events.filter((event) => event.processing_error || ["failed", "rejected"].includes(event.normalized_status)).length ? <div className="table-wrap"><table><thead><tr><th>Event</th><th>Status</th><th>When</th><th>Safe detail</th></tr></thead><tbody>{events.filter((event) => event.processing_error || ["failed", "rejected"].includes(event.normalized_status)).slice(0, 10).map((event) => <tr key={event.id}><td>{humanStatus(event.event_type)}</td><td><Badge tone="danger">{humanStatus(event.normalized_status)}</Badge></td><td>{relativeTime(event.received_at)}</td><td>{event.processing_error || "Provider rejected the request"}</td></tr>)}</tbody></table></div> : <EmptyState title="No provider errors" body="Meta and transport failures will appear separately from messaging-policy blocks." />}
				</section>
                {canManage ? <div className="whatsapp-connection-controls">
                  {selected.status === "disconnected" ? <Button icon={RefreshCw} loading={saving} onClick={() => void setConnectionStatus("connecting")}>Reconnect</Button> : ["connected", "healthy", "degraded", "requires_attention"].includes(selected.status) ? <Button icon={Unplug} loading={saving} onClick={() => void setConnectionStatus("disconnected")}>Disconnect</Button> : null}
                </div> : null}
                {!canManage ? <p className="read-only-note">WhatsApp onboarding is read-only for your role.</p> : null}
              </Card> : null}

              <div className="metrics compact-metrics" id={selected.provider === "whatsapp" ? "whatsapp-analytics" : undefined}>
                <Metric label="Inbound messages" value={detail.metrics.inbound_count} hint={`${detail.metrics.days} reporting days`} />
                <Metric label="Outbound messages" value={detail.metrics.outbound_count} hint={`${detail.metrics.failed_outbound_count} failed`} />
                <Metric label="Conversations" value={detail.metrics.conversations_started} hint={`${detail.metrics.conversations_human_handled} human handled`} />
                <Metric label="Response time" value={detail.metrics.average_response_ms ? `${detail.metrics.average_response_ms} ms` : "—"} hint="Provider-reported average" />
              </div>
			  {selected.provider === "whatsapp" && whatsAppMetrics ? <div className="metrics compact-metrics whatsapp-advanced-metrics">
				<Metric label="Free-form sends" value={whatsAppMetrics.freeform_sends} hint={`${whatsAppMetrics.template_sends} template sends`} />
				<Metric label="Delivery" value={whatsAppMetrics.delivered} hint={`${whatsAppMetrics.read} read`} />
				<Metric label="Policy blocks" value={whatsAppMetrics.policy_blocked_sends} hint={`${whatsAppMetrics.service_window_closed_blocks} closed-window`} />
				<Metric label="Provider errors" value={whatsAppMetrics.provider_errors} hint={`${whatsAppMetrics.rate_limits} rate limits`} />
				<Metric label="Handoff rate" value={`${Math.round(whatsAppMetrics.handoff_rate * 100)}%`} hint={`${whatsAppMetrics.human_handled_conversations} human handled`} />
				<Metric label="Delivery latency" value={whatsAppMetrics.average_provider_delivery_ms ? `${whatsAppMetrics.average_provider_delivery_ms} ms` : "—"} hint="Provider delivery average" />
			  </div> : null}

              <div className="channel-detail-grid">
                <Card id={selected.provider === "whatsapp" ? "whatsapp-health" : undefined}>
                  <SectionHeader title="Health" description="Latest adapter and provider checks" />
                  <p className="channel-section-status"><Activity size={18} aria-hidden="true" /><Badge tone={badgeTone(detail.health.status)}>{humanStatus(detail.health.status)}</Badge></p>
                  {detail.health.last_check ? <div className="channel-event-row"><span><strong>{detail.health.last_check.error_code || "Latest check"}</strong><small>{relativeTime(detail.health.last_check.checked_at)} · {detail.health.last_check.latency_ms} ms</small></span><span>{detail.health.last_check.error_message || "No provider error reported."}</span></div> : <EmptyState title="No health checks yet" body="Health checks begin after a provider adapter is configured." />}
                </Card>
                <Card>
                  <SectionHeader title="Credentials" description="References and status only; secret values are never returned" />
                  {detail.credentials.length ? detail.credentials.map((credential) => <div className="channel-event-row" key={credential.id}><span><strong>{humanStatus(credential.credential_type)}</strong><small>{credential.reference_type ? `${humanStatus(credential.reference_type)} reference` : "No reference"}</small></span><Badge tone={badgeTone(credential.status)}>{humanStatus(credential.status)}</Badge></div>) : <EmptyState title="No credential references" body="Add credentials in the provider setup panel." />}
                </Card>
              </div>

              <Card id={selected.provider === "whatsapp" ? "whatsapp-activity" : undefined}>
                <SectionHeader title="Provider identity" description="Accounts and numbers attached to this connection" />
                {detail.provider_accounts.length === 0 && detail.identities.length === 0 ? <EmptyState title="No provider identity" body="An external provider account has not been authorized or provisioned." /> : <div className="channel-identity-list">
                  {detail.provider_accounts.map((account) => <div className="channel-event-row" key={account.id}><span><strong>{account.display_name || account.business_name || "Provider account"}</strong><small>Account</small></span><Badge tone={badgeTone(account.status)}>{humanStatus(account.status)}</Badge></div>)}
                  {detail.identities.map((identity) => <div className="channel-event-row" key={identity.id}><span><strong>{identity.identity_type === "phone_number" ? maskPhone(identity.display_name || identity.external_handle, "Connected number") : identity.display_name || identity.external_handle || "Channel identity"}</strong><small>{humanStatus(identity.identity_type)}</small></span><Badge tone={badgeTone(identity.status)}>{humanStatus(identity.status)}</Badge></div>)}
                </div>}
              </Card>

              <Card>
                <SectionHeader title="Provider events" description="Normalized setup, delivery, and provider activity" />
                {selected.provider === "whatsapp" && whatsApp ? (whatsApp.operational_events.length ? <div className="table-wrap"><table><thead><tr><th>Event</th><th>Status</th><th>Received</th><th>Next step</th></tr></thead><tbody>{whatsApp.operational_events.map((event, index) => <tr key={`${event.type}-${event.status}-${event.occurred_at}-${index}`}><td>{event.title}</td><td><Badge tone={badgeTone(event.status)}>{humanStatus(event.status)}</Badge></td><td>{relativeTime(event.occurred_at)}</td><td>{event.guidance || "No action required"}</td></tr>)}</tbody></table></div> : <EmptyState title="No WhatsApp activity" body="Verification, messages, and delivery updates will appear here." />) : events.length ? <div className="table-wrap"><table><thead><tr><th>Event</th><th>Status</th><th>Received</th><th>Note</th></tr></thead><tbody>{events.map((event) => <tr key={event.id}><td>{humanStatus(event.event_type)}</td><td><Badge tone={badgeTone(event.normalized_status)}>{humanStatus(event.normalized_status)}</Badge></td><td>{relativeTime(event.received_at)}</td><td>{event.processing_error || "—"}</td></tr>)}</tbody></table></div> : <EmptyState title="No provider events" body="Events will appear after an adapter receives normalized provider activity." />}
              </Card>

              {canManage ? <Card id={selected.provider === "whatsapp" ? "whatsapp-settings" : undefined}>
                <SectionHeader title="Connection settings" description="These settings do not authorize or connect an external provider." />
                <FormGrid onSubmit={saveConnection}>
                  <label>Display name<input value={editForm.display_name} onChange={(event) => setEditForm({ ...editForm, display_name: event.target.value })} required /></label>
                  <label>Ownership<select value={editForm.ownership_model} onChange={(event) => setEditForm({ ...editForm, ownership_model: event.target.value })}><option value="merchant_managed">Merchant-managed</option><option value="zidi_managed">Zidi-managed</option></select></label>
                  <label>Environment<select value={editForm.environment} onChange={(event) => setEditForm({ ...editForm, environment: event.target.value })}><option value="sandbox">Sandbox</option><option value="production">Production</option></select></label>
                  <div className="full channel-settings-actions"><Button type="submit" icon={ShieldCheck} loading={saving} className="primary">Save settings</Button>{selected.status !== "archived" ? <ConfirmButton label="Archive connection" confirm="This removes the connection from active channel setup. Conversation and audit history remain available." onConfirm={() => void archiveConnection()} tone="danger" /> : null}</div>
                </FormGrid>
              </Card> : <p className="read-only-note">Channel configuration is read-only for your role.</p>}
            </>}
          </div>
        </div>
      )}

      <Modal open={createOpen} title="Prepare channel setup" description="Create a foundation record without connecting an external provider." onClose={() => setCreateOpen(false)} footer={<><button type="button" className="ghost" onClick={() => setCreateOpen(false)}>Cancel</button><Button type="submit" form="channel-create-form" icon={Plus} className="primary" loading={saving}>Create setup record</Button></>}>
        <form id="channel-create-form" className="form-grid" onSubmit={createConnection}>
          <label>Provider<select value={createForm.provider} onChange={(event) => { const provider = providers.find((item) => item.key === event.target.value); setCreateForm({ ...createForm, provider: event.target.value, display_name: provider?.label || event.target.value }); }}>{providers.map((provider) => <option key={provider.key} value={provider.key}>{provider.label}</option>)}</select></label>
          <label>Display name<input value={createForm.display_name} onChange={(event) => setCreateForm({ ...createForm, display_name: event.target.value })} required /></label>
          <label>Ownership<select value={createForm.ownership_model} onChange={(event) => setCreateForm({ ...createForm, ownership_model: event.target.value })}><option value="merchant_managed">Merchant-managed</option><option value="zidi_managed">Zidi-managed</option></select></label>
          <label>Environment<select value={createForm.environment} onChange={(event) => setCreateForm({ ...createForm, environment: event.target.value })}><option value="sandbox">Sandbox</option><option value="production">Production</option></select></label>
        </form>
      </Modal>

	  <Modal open={templateOpen} title={editingTemplateID ? "Edit WhatsApp template" : "Add WhatsApp template"} description="Mirror the exact name, language, content, and status shown in Meta. Local approval does not submit a template to Meta." onClose={() => setTemplateOpen(false)} footer={<><button type="button" className="ghost" onClick={() => setTemplateOpen(false)}>Cancel</button><Button type="submit" form="whatsapp-template-form" icon={ShieldCheck} className="primary" loading={saving}>Save template</Button></>}>
		<form id="whatsapp-template-form" className="form-grid" onSubmit={saveTemplate}>
		  <label>Template name<input value={templateForm.name} onChange={(event) => setTemplateForm({ ...templateForm, name: event.target.value.toLowerCase().replace(/[^a-z0-9_]/g, "_") })} placeholder="order_ready" required /></label>
		  <label>Language<input value={templateForm.language} onChange={(event) => setTemplateForm({ ...templateForm, language: event.target.value })} placeholder="en" required /></label>
		  <label>Category<select value={templateForm.category} onChange={(event) => setTemplateForm({ ...templateForm, category: event.target.value })}><option value="utility">Utility</option><option value="authentication">Authentication</option><option value="marketing">Marketing</option><option value="service">Service</option></select></label>
		  <label>Status<select value={templateForm.status} onChange={(event) => setTemplateForm({ ...templateForm, status: event.target.value })}><option value="draft">Draft</option><option value="pending">Pending</option><option value="approved">Approved in Meta</option><option value="rejected">Rejected</option><option value="paused">Paused</option><option value="disabled">Disabled</option><option value="archived">Archived</option></select></label>
		  <label className="full">Body<textarea rows={5} maxLength={4096} value={templateForm.body} onChange={(event) => setTemplateForm({ ...templateForm, body: event.target.value })} placeholder="Hi {{1}}, your order {{2}} is ready." required /></label>
		  <label>Variables<input value={templateForm.variables} onChange={(event) => setTemplateForm({ ...templateForm, variables: event.target.value })} placeholder="name, order_code" /></label>
		  <label>Sample values<input value={templateForm.sample_values} onChange={(event) => setTemplateForm({ ...templateForm, sample_values: event.target.value })} placeholder="name=Ada, order_code=ZC-100" /></label>
		  <label>Meta template reference<input value={templateForm.provider_template_id} onChange={(event) => setTemplateForm({ ...templateForm, provider_template_id: event.target.value })} autoComplete="off" /></label>
		  <label>Rejection note<input value={templateForm.rejection_reason} onChange={(event) => setTemplateForm({ ...templateForm, rejection_reason: event.target.value })} /></label>
		  <div className="full whatsapp-template-preview"><small>Preview</small><p>{templatePreview() || "Template preview appears here."}</p></div>
		</form>
	  </Modal>
    </Page>
  );
}
