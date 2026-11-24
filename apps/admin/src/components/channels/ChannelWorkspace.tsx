import { WhatsAppPhone } from "../whatsapp/WhatsAppPhone";
import { WhatsAppThread } from "../whatsapp/WhatsAppThread";
import { Activity, Archive, Check, Clipboard, Headphones, KeyRound, Link2, Pencil, Plus, RefreshCw, Send, ShieldCheck, Unplug } from "lucide-react";
import { Badge, Button, Card, ConfirmButton, EmptyState, Flash, FormGrid, IconButton, LoadingState, Metric, Modal, Page, SectionHeader } from "../ui";
import { OAuthChannelPanel, oauthConnectionFromSession, type OAuthChannelDetail } from "./OAuthChannelPanel";
import { humanStatus, maskPhone, relativeTime } from "../../lib/format";
import { Link } from "react-router-dom";
import { ChannelsHub } from "./ChannelsHub";
import { ChannelExperience } from "./ChannelExperience";
import { channelStatus, capabilityLabel } from "../../lib/channel-status";
import "./channels.css";
import type { ChannelConnection } from "./channel-types";
import { useChannelWorkspace, providers, providerLabel, badgeTone, ReadinessItem, setupSteps } from "./useChannelWorkspace";

export function ChannelWorkspace() {
  const {
    canView, detailError, tab, setTab, wizard, setWizard, canManage, connections, selectedID, detail, events, whatsApp, whatsAppForm, setWhatsAppForm, whatsAppTemplates, whatsAppContacts,
    whatsAppMetrics, policyBlocks, templateOpen, setTemplateOpen, editingTemplateID, templateForm, setTemplateForm, credentialMode, setCredentialMode, credentialForm, setCredentialForm, testForm, setTestForm, loading, detailLoading, createOpen,
    setCreateOpen, createForm, setCreateForm, editForm, setEditForm, saving, connectingMeta, message, setMessage, success, setSuccess, loadConnections, loadDetail, beginCreate, createConnection, saveConnection,
    archiveConnection, saveWhatsApp, rotateWhatsAppCredential, sendWhatsAppTest, beginCreateTemplate, beginEditTemplate, saveTemplate, archiveTemplate, templatePreview, completeWhatsAppSetup, migrateLegacyCredentials, checkWhatsAppHealth, connectWithMeta, requestZidiHelp, updateAssistedStatus, setConnectionStatus, copyWebhookURL, selected, webhookURL, user, connectionId
  } = useChannelWorkspace();
  if (!canView) return <Page title="Channels"><EmptyState title="Channel access required" body="Your role does not have permission to view channels." /></Page>;
  return (
    <Page
      title={connectionId ? detail?.connection.display_name || "Channel" : "Channels"}
      description="Connect your business to the conversations that matter."
      help="Connect a channel, check its health and manage who can message your business."
      actions={canManage ? <Button icon={Plus} className="primary" onClick={() => beginCreate(providers[0])}>Connect WhatsApp</Button> : undefined}
    >
      <Flash message={message} />
      <Flash message={success} tone="success" />

      {!connectionId && !oauthConnectionFromSession() ? <ChannelsHub canManage={canManage} onConnect={() => beginCreate(providers[0])} onConnectProvider={key => { const provider = providers.find(item => item.key === key); if (provider) beginCreate(provider); }} /> : (
        <div className="channels-detail">
          <Link to="/settings/whatsapp">← All channels</Link>
          <nav className="channels-tabs" aria-label="Channel details">{["overview", "experience", "test", "health", "settings"].map(value => <button key={value} type="button" aria-current={tab === value ? "page" : undefined} onClick={() => { setTab(value); setWizard(0); }}>{humanStatus(value)}</button>)}</nav>
          {wizard > 0 ? <section className="channels-wizard" aria-label="Connect WhatsApp">
            <ol>{["Connection method", "Details", "Verify webhook and health", "Set experience"].map((label, index) => <li key={label}><button type="button" aria-current={wizard === index + 1 ? "step" : undefined} onClick={() => { setWizard(index + 1); setTab(index === 3 ? "experience" : index === 2 ? "health" : "settings"); }}>{index + 1}. {label}</button></li>)}</ol>
            {wizard === 1 ? <><h2>Choose how to connect</h2><Button disabled={!canManage || !whatsApp?.embedded_signup_available} loading={connectingMeta} onClick={() => void connectWithMeta()}>Connect with Meta</Button>{!whatsApp?.embedded_signup_available ? <p>Requires Meta Embedded Signup configuration</p> : null}<Button disabled={!canManage || Boolean(selected && user.role !== "platform_admin" && selected.ownership_model !== "zidi_managed" && !whatsApp?.legacy_credentials_found)} onClick={() => { setWizard(2); setTab("settings"); }}>Enter credentials manually</Button>{selected && user.role !== "platform_admin" && selected.ownership_model !== "zidi_managed" && !whatsApp?.legacy_credentials_found ? <p>Manual recovery requires an operator or an assisted connection. Choose Zidi under Managed by when creating an assisted connection, or ask your administrator for help.</p> : null}</> : null}
            {wizard > 1 && wizard < 4 ? <Button onClick={() => { setWizard(wizard + 1); setTab(wizard === 2 ? "health" : "experience"); }}>{wizard === 2 ? "Verify connection" : "Continue to experience"}</Button> : wizard === 4 ? <Button onClick={() => { setWizard(0); setTab("experience"); }}>Done — set experience</Button> : null}
          </section> : null}
          <div className="channel-detail-stack">
            {detailLoading ? <Card><LoadingState label="Loading channel details" /></Card> : !detail || !selected ? <Card><EmptyState title={detailError ? "Could not load this channel" : "Loading channel"} body={detailError || "Fetching this connection’s details."} action={<Button onClick={() => void loadDetail(selectedID || connectionId || "")}>Retry</Button>} /></Card> : <>
              {tab === "overview" ? <Card>
                <SectionHeader title={selected.display_name} description={providerLabel(selected.provider)} action={<Button onClick={() => void loadDetail(selected.id)}>Refresh</Button>} />
                <p><Badge tone={channelStatus(selected).tone}>{channelStatus(selected).label}</Badge> {channelStatus(selected, whatsApp?.next_action || detail.health.last_check?.error_message).reason}</p>
                <dl className="channels-facts">
                  {detail.provider_accounts[0]?.business_name ? <><dt>Business</dt><dd>{detail.provider_accounts[0].business_name}</dd></> : null}
                  {whatsApp?.display_phone_number ? <><dt>Number</dt><dd>{whatsApp.display_phone_number}</dd></> : null}
                  {whatsApp?.whatsapp_business_account_id ? <><dt>WhatsApp business account</dt><dd>••••{whatsApp.whatsapp_business_account_id.slice(-4)}</dd></> : null}
                  <dt>Environment</dt><dd>{selected.environment === "production" ? "Live" : humanStatus(selected.environment)}</dd>
                  <dt>Last health check</dt><dd>{selected.last_health_check_at ? relativeTime(selected.last_health_check_at) : "Not checked yet"}</dd>
                </dl>
                {whatsApp?.next_action ? <p>{whatsApp.next_action}</p> : null}
                {canManage && selected.provider === "whatsapp" && channelStatus(selected).label === "Setup incomplete" ? <Button onClick={() => { setWizard(1); setTab("settings"); }}>Continue setup</Button> : null}
                <Button onClick={() => setTab("health")}>Review health</Button><Button onClick={() => setTab("experience")}>View experience</Button>
              </Card> : null}
              {tab === "health" ? <Card>
                <SectionHeader title="What this channel can do" description="Readiness from the latest provider checks." action={selected.provider === "whatsapp" && canManage ? <Button loading={saving} onClick={() => void checkWhatsAppHealth()}>Refresh health</Button> : <Button onClick={() => void loadDetail(selected.id)}>Refresh</Button>} />
                <p>{channelStatus(selected, detail.health.last_check?.error_message).reason}</p>
                {selected.capability_states?.length ? selected.capability_states.map(item => <div className="channel-event-row" key={item.capability}><span><strong>{capabilityLabel(item.capability)}</strong><small>{item.reason || (item.status === "available" ? "Ready to use" : "Provider readiness has not been confirmed")}</small></span><Badge tone={item.status === "available" ? "success" : "neutral"}>{item.status === "available" ? "Ready" : humanStatus(item.status)}</Badge></div>) : <p>Capability checks are not available for this connection. Refresh health after completing setup.</p>}
                {whatsApp?.last_authorization_error ? <p>{whatsApp.last_authorization_error}</p> : null}
                {whatsApp?.last_webhook_verification_error ? <p>{whatsApp.last_webhook_verification_error}</p> : null}
                {whatsApp?.credentials.filter(item => item.resolution_problem).map(item => <p key={item.credential_type}>{humanStatus(item.credential_type)}: {item.resolution_problem}</p>)}
                {canManage ? <Button onClick={() => setTab("settings")}>Fix connection settings</Button> : null}
              </Card> : null}
              {tab === "experience" ? <ChannelExperience connectionId={selected.id} /> : null}
              {selected.provider === "whatsapp" && whatsApp && ["settings", "health", "test"].includes(tab) ? <Card className={`whatsapp-setup-panel channels-tab-${tab}`}>
                <SectionHeader title="WhatsApp" description="Review delivery readiness and manage this number." action={canManage ? <Button icon={Activity} onClick={() => void checkWhatsAppHealth()} loading={saving}>Check health</Button> : undefined} />
                {whatsApp.authorization_status !== "authorized" ? <section className="whatsapp-connect-panel" hidden={tab !== "settings"}>
                  <div>
                    <h3>Connect WhatsApp</h3>
                    <p>Meta handles sign-in, business selection, phone selection, and authorization. Zidi stores the resulting connection securely.</p>
                  </div>
                  {canManage ? <div className="whatsapp-connect-actions">
                    <Button icon={Link2} className="primary" loading={connectingMeta} disabled={!whatsApp.embedded_signup_available} onClick={() => void connectWithMeta()}>Connect with Meta</Button>
                    <Button icon={Headphones} loading={saving} onClick={() => void requestZidiHelp()}>Let Zidi help</Button>
                  </div> : null}
                  {!whatsApp.embedded_signup_available ? <p className="whatsapp-config-warning">Requires Meta Embedded Signup configuration</p> : null}
                </section> : <section className="whatsapp-connected-summary" hidden={tab !== "settings"}>
                  <div><span>Business</span><strong>{detail.provider_accounts[0]?.business_name || detail.provider_accounts[0]?.display_name || "WhatsApp Business"}</strong></div>
                  <div><span>Phone</span><strong>{maskPhone(whatsApp.display_phone_number, "Connected number")}</strong></div>
                  <div><span>Managed by</span><strong>{selected.ownership_model === "zidi_managed" ? "Zidi" : "You"}</strong></div>
                  <div><span>Authorized</span><strong>{whatsApp.authorized_at ? relativeTime(whatsApp.authorized_at) : "Connected"}</strong></div>
                </section>}

                {whatsApp.assisted_setup_status !== "not_requested" ? <div className="whatsapp-assisted-status" hidden={tab !== "settings"}>
                  <span><small>Assisted setup</small><Badge tone={badgeTone(whatsApp.assisted_setup_status)}>{humanStatus(whatsApp.assisted_setup_status)}</Badge></span>
                  <p>{whatsApp.assisted_setup_note || (whatsApp.assisted_setup_status === "awaiting_merchant_action" ? "Your action is required in Meta before setup can continue." : "Zidi is tracking the remaining Meta setup steps.")}</p>
                  {user.role === "platform_admin" && !["completed", "connected"].includes(whatsApp.assisted_setup_status) ? <div className="channel-settings-actions">
                    <Button icon={RefreshCw} loading={saving} onClick={() => void updateAssistedStatus("in_progress")}>In progress</Button>
                    <Button icon={Headphones} loading={saving} onClick={() => void updateAssistedStatus("awaiting_merchant_action")}>Awaiting merchant</Button>
                  </div> : null}
                </div> : null}

                <section className="whatsapp-stage" id="whatsapp-connection" hidden={tab !== "health"}>
                  <SectionHeader title="Connection" description="Meta authorization and Zidi readiness are tracked separately" />
                  <div className="whatsapp-readiness">
                    <div><span>Inbound</span><Badge tone={whatsApp.checklist.ready_for_inbound ? "success" : "warning"}>{whatsApp.checklist.ready_for_inbound ? "Ready" : "Setup required"}</Badge></div>
                    <div><span>Outbound</span><Badge tone={whatsApp.checklist.ready_for_outbound ? "success" : "warning"}>{whatsApp.checklist.ready_for_outbound ? "Ready" : "Setup required"}</Badge></div>
                    <div><span>Webhook</span><Badge tone={badgeTone(whatsApp.webhook_status)}>{humanStatus(whatsApp.webhook_status)}</Badge></div>
                    <div><span>Signed events</span><Badge tone={badgeTone(whatsApp.signature_status)}>{humanStatus(whatsApp.signature_status)}</Badge></div>
                  </div>
                </section>

                {(user.role === "platform_admin" || selected.ownership_model === "zidi_managed" || whatsApp.legacy_credentials_found) ? <section className="whatsapp-stage whatsapp-operator-panel" hidden={tab !== "settings"}>
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

                <section className="whatsapp-stage" id="whatsapp-webhooks" hidden={tab !== "settings" && tab !== "health"}>
                  <SectionHeader title="Webhooks" description={whatsApp.connection_method === "embedded_signup" ? "Zidi subscribes the authorized account to its shared Meta app webhook" : "Manual connections retain their connection-specific callback"} />
                  <div className="whatsapp-webhook-row"><span><small>Callback URL</small><code>{webhookURL}</code></span><IconButton label="Copy webhook URL" icon={Clipboard} onClick={() => void copyWebhookURL()} /></div>
                  <div className="whatsapp-checklist"><ReadinessItem complete={whatsApp.checklist.verify_token_configured} label="Verification configured" /><ReadinessItem complete={whatsApp.checklist.webhook_verified} label="WABA subscribed" /><ReadinessItem complete={whatsApp.checklist.signature_verified} label="Signed event observed" /></div>
                  <div className="whatsapp-timestamps"><span><small>Last verification</small><strong>{whatsApp.last_webhook_verification_attempt_at ? relativeTime(whatsApp.last_webhook_verification_attempt_at) : "Not attempted"}</strong></span><span><small>Last valid signed webhook</small><strong>{whatsApp.last_signature_verified_at ? relativeTime(whatsApp.last_signature_verified_at) : "Not observed"}</strong></span><span><small>Last rejected attempt</small><strong>{whatsApp.last_signature_rejected_at ? relativeTime(whatsApp.last_signature_rejected_at) : "None"}</strong></span><span><small>Rejected attempts</small><strong>{whatsApp.rejected_signature_count || 0}</strong></span><span><small>Last inbound event</small><strong>{whatsApp.last_inbound_at ? relativeTime(whatsApp.last_inbound_at) : "None"}</strong></span></div>
                </section>

                <section className="whatsapp-stage" id="whatsapp-messaging" hidden={tab !== "test"}>
				  <SectionHeader title="Messaging check" description="Free-form tests require an open service window; an approved template is required otherwise" />
                  <div className="whatsapp-checklist">
                    <ReadinessItem complete={whatsApp.checklist.test_message_ready} label="Ready to test" />
                    <ReadinessItem complete={whatsApp.checklist.test_message_sent} label="Outbound accepted" />
                    <ReadinessItem complete={whatsApp.checklist.inbound_test_received} label="Inbound reply received" />
                  </div>
                  <p>This is a message preview, not a live conversation. Use a designated tester phone to message this number, then check for a reply. Tester restriction status is not returned by the API. The server enforces any configured recipient restrictions.</p>
                  <WhatsAppPhone businessName={selected.display_name}><WhatsAppThread lines={[{ from: "bot", message: { type: "text", text: testForm.message } }]} /></WhatsAppPhone>
                  <p>WhatsApp Flow tests require the designated sandbox WABA and phone configured on the server. This API does not identify which account is designated. The connection’s configured number is shown in Overview; Flow publishing retains its server-side checks.</p>
                  {whatsApp.last_test_message_error ? <Flash message={whatsApp.last_test_message_error} /> : null}
                  {canManage ? <form className="form-grid whatsapp-test-form" onSubmit={sendWhatsAppTest}>
                    <label>Approved test recipient<input inputMode="tel" autoComplete="off" value={testForm.recipient} onChange={(event) => setTestForm({ ...testForm, recipient: event.target.value })} placeholder="+2348000000000" required /></label>
					<label>Send mode<select value={testForm.template_id} onChange={(event) => setTestForm({ ...testForm, template_id: event.target.value, template_variables: {} })}><option value="">Free-form service reply</option>{whatsAppTemplates.filter((template) => template.status === "approved").map((template) => <option key={template.id} value={template.id}>{template.name} · {template.language}</option>)}</select></label>
					{testForm.template_id ? whatsAppTemplates.find((template) => template.id === testForm.template_id)?.variable_schema.map((name) => <label key={name}>{humanStatus(name)}<input value={testForm.template_variables[name] || ""} onChange={(event) => setTestForm({ ...testForm, template_variables: { ...testForm.template_variables, [name]: event.target.value } })} required /></label>) : <label>Test message<input value={testForm.message} onChange={(event) => setTestForm({ ...testForm, message: event.target.value })} maxLength={1000} required /></label>}
                    <div className="full channel-settings-actions"><Button type="submit" icon={Send} className="primary" loading={saving} disabled={!whatsApp.checklist.test_message_ready}>Send test message</Button>{whatsApp.checklist.ready_to_complete ? <Button type="button" icon={ShieldCheck} loading={saving} onClick={() => void completeWhatsAppSetup()}>Complete validation</Button> : null}</div>
                  </form> : null}
                  {whatsApp.test_recipient_display ? <p className="whatsapp-test-result">Latest approved recipient: <strong>{whatsApp.test_recipient_display}</strong> · outbound {whatsApp.last_test_message_at ? relativeTime(whatsApp.last_test_message_at) : "not sent"} · inbound {whatsApp.last_inbound_test_at ? relativeTime(whatsApp.last_inbound_test_at) : "awaiting reply"}</p> : null}
                </section>

				<section className="whatsapp-stage" id="whatsapp-policy" hidden={tab !== "test"}>
				  <SectionHeader title="Messaging policy" description="Zidi evaluates consent and the 24-hour customer-service window before every Meta request" />
				  <div className="whatsapp-checklist"><ReadinessItem complete label="Free-form inside open window" /><ReadinessItem complete={whatsAppTemplates.some((template) => template.status === "approved")} label="Approved template available" /><ReadinessItem complete={(whatsAppMetrics?.opted_out_contacts || 0) === 0} label="No opted-out contacts" /></div>
				  <div className="whatsapp-timestamps"><span><small>Open windows</small><strong>{whatsAppMetrics?.service_window_open_contacts || 0}</strong></span><span><small>Opted in</small><strong>{whatsAppMetrics?.opted_in_contacts || 0}</strong></span><span><small>Opted out</small><strong>{whatsAppMetrics?.opted_out_contacts || 0}</strong></span><span><small>Policy blocks</small><strong>{whatsAppMetrics?.policy_blocked_sends || 0}</strong></span></div>
				</section>

				<section className="whatsapp-stage" id="whatsapp-templates" hidden={tab !== "test"}>
				  <SectionHeader title="Templates" description="Local mirrors of Meta templates; only records marked approved can send outside the service window" action={canManage ? <Button icon={Plus} onClick={beginCreateTemplate}>Add template</Button> : undefined} />
				  {whatsAppTemplates.length ? <div className="table-wrap"><table><thead><tr><th>Template</th><th>Category</th><th>Status</th><th>Updated</th>{canManage ? <th><span className="sr-only">Actions</span></th> : null}</tr></thead><tbody>{whatsAppTemplates.map((template) => <tr key={template.id}><td><strong>{template.name}</strong><small>{template.language} · {template.variable_schema.length} variables</small></td><td>{humanStatus(template.category)}</td><td><Badge tone={badgeTone(template.status)}>{humanStatus(template.status)}</Badge></td><td>{relativeTime(template.updated_at)}</td>{canManage ? <td><div className="table-actions"><IconButton label={`Edit ${template.name}`} icon={Pencil} onClick={() => beginEditTemplate(template)} />{template.status !== "archived" ? <IconButton label={`Archive ${template.name}`} icon={Archive} onClick={() => { if (window.confirm(`Archive ${template.name}? It will no longer be available for sending.`)) void archiveTemplate(template.id); }} /> : null}</div></td> : null}</tr>)}</tbody></table></div> : <EmptyState title="No template records" body="Mirror an approved Meta template here before sending outside a customer-service window." />}
				</section>

				<section className="whatsapp-stage" id="whatsapp-contacts" hidden={tab !== "test"}>
				  <SectionHeader title="Contact consent and service windows" description="Identifiers remain masked; inbound messages extend the window for 24 hours" />
				  {whatsAppContacts.length ? <div className="table-wrap"><table><thead><tr><th>Contact</th><th>Consent</th><th>Service window</th><th>Last inbound</th></tr></thead><tbody>{whatsAppContacts.map((contact) => { const open = Boolean(contact.service_window_expires_at && new Date(contact.service_window_expires_at) > new Date()); return <tr key={contact.id}><td>{contact.masked_phone}</td><td><Badge tone={contact.consent_status === "opted_out" ? "danger" : contact.consent_status === "opted_in" ? "success" : "neutral"}>{humanStatus(contact.consent_status)}</Badge></td><td><Badge tone={open ? "success" : "warning"}>{open ? "Open" : "Closed"}</Badge>{contact.service_window_expires_at ? <small>{open ? `until ${relativeTime(contact.service_window_expires_at)}` : `expired ${relativeTime(contact.service_window_expires_at)}`}</small> : null}</td><td>{contact.last_inbound_at ? relativeTime(contact.last_inbound_at) : "None"}</td></tr>; })}</tbody></table></div> : <EmptyState title="No WhatsApp contact state" body="Contact consent and service-window state appears after a signed inbound message or approved template send." />}
				</section>

				<section className="whatsapp-stage" hidden={tab !== "health"}>
				  <SectionHeader title="Recent policy blocks" description="Messages stopped before Meta was called" />
				  {policyBlocks.length ? <div className="table-wrap"><table><thead><tr><th>Reason</th><th>Message</th><th>When</th></tr></thead><tbody>{policyBlocks.map((block) => <tr key={block.id}><td><Badge tone="warning">{humanStatus(block.decision)}</Badge><small>{block.reason}</small></td><td>{humanStatus(block.message_type)}</td><td>{relativeTime(block.created_at)}</td></tr>)}</tbody></table></div> : <EmptyState title="No policy blocks" body="No WhatsApp message has been blocked by consent, window, or template policy." />}
				</section>

				<section className="whatsapp-stage" hidden={tab !== "health"}>
				  <SectionHeader title="Recent provider errors" description="Meta or transport failures after policy allowed a request" />
				  {events.filter((event) => event.processing_error || ["failed", "rejected"].includes(event.normalized_status)).length ? <div className="table-wrap"><table><thead><tr><th>Event</th><th>Status</th><th>When</th><th>Safe detail</th></tr></thead><tbody>{events.filter((event) => event.processing_error || ["failed", "rejected"].includes(event.normalized_status)).slice(0, 10).map((event) => <tr key={event.id}><td>{humanStatus(event.event_type)}</td><td><Badge tone="danger">{humanStatus(event.normalized_status)}</Badge></td><td>{relativeTime(event.received_at)}</td><td>{event.processing_error || "Provider rejected the request"}</td></tr>)}</tbody></table></div> : <EmptyState title="No provider errors" body="Meta and transport failures will appear separately from messaging-policy blocks." />}
				</section>
                {canManage ? <div className="whatsapp-connection-controls" hidden={tab !== "settings"}>
                  {selected.status === "disconnected" ? <Button icon={RefreshCw} loading={saving} onClick={() => void setConnectionStatus("connecting")}>Reconnect</Button> : ["connected", "healthy", "degraded", "requires_attention"].includes(selected.status) ? <Button icon={Unplug} loading={saving} onClick={() => void setConnectionStatus("disconnected")}>Disconnect</Button> : null}
                </div> : null}
                {!canManage ? <p className="read-only-note">WhatsApp onboarding is read-only for your role.</p> : null}
              </Card> : null}

              {tab === "settings" && ["instagram", "facebook", "tiktok"].includes(selected.provider) ? <Card>
                <OAuthChannelPanel
                  detail={detail as OAuthChannelDetail}
                  events={events}
                  canManage={canManage}
                  onChanged={async () => {
                    await loadConnections(selected.id);
                    await loadDetail(selected.id);
                  }}
                  onMessage={(value, succeeded) => {
                    if (succeeded) {
                      setMessage("");
                      setSuccess(value);
                    } else {
                      setSuccess("");
                      setMessage(value);
                    }
                  }}
                />
              </Card> : null}

              <div hidden={tab !== "health"} className="metrics compact-metrics" id={selected.provider === "whatsapp" ? "whatsapp-analytics" : undefined}>
                <Metric label="Inbound messages" value={detail.metrics.inbound_count} hint={`${detail.metrics.days} reporting days`} />
                <Metric label="Outbound messages" value={detail.metrics.outbound_count} hint={`${detail.metrics.failed_outbound_count} failed`} />
                <Metric label="Conversations" value={detail.metrics.conversations_started} hint={`${detail.metrics.conversations_human_handled} human handled`} />
                <Metric label="Response time" value={detail.metrics.average_response_ms ? `${detail.metrics.average_response_ms} ms` : "—"} hint="Provider-reported average" />
              </div>
			  {selected.provider === "whatsapp" && whatsAppMetrics ? <div hidden={tab !== "health"} className="metrics compact-metrics whatsapp-advanced-metrics">
				<Metric label="Free-form sends" value={whatsAppMetrics.freeform_sends} hint={`${whatsAppMetrics.template_sends} template sends`} />
				<Metric label="Delivery" value={whatsAppMetrics.delivered} hint={`${whatsAppMetrics.read} read`} />
				<Metric label="Policy blocks" value={whatsAppMetrics.policy_blocked_sends} hint={`${whatsAppMetrics.service_window_closed_blocks} closed-window`} />
				<Metric label="Provider errors" value={whatsAppMetrics.provider_errors} hint={`${whatsAppMetrics.rate_limits} rate limits`} />
				<Metric label="Handoff rate" value={`${Math.round(whatsAppMetrics.handoff_rate * 100)}%`} hint={`${whatsAppMetrics.human_handled_conversations} human handled`} />
				<Metric label="Delivery latency" value={whatsAppMetrics.average_provider_delivery_ms ? `${whatsAppMetrics.average_provider_delivery_ms} ms` : "—"} hint="Provider delivery average" />
			  </div> : null}

              <div className="channel-detail-grid" hidden={tab !== "health"}>
                <Card id={selected.provider === "whatsapp" ? "whatsapp-health" : undefined}>
                  <SectionHeader title="Health" description="Latest adapter and provider checks" />
                  <p className="channel-section-status"><Activity size={18} aria-hidden="true" /><Badge tone={badgeTone(detail.health.status)}>{humanStatus(detail.health.status)}</Badge></p>
                  {detail.health.last_check ? <div className="channel-event-row"><span><strong>{detail.health.last_check.error_code || "Latest check"}</strong><small>{relativeTime(detail.health.last_check.checked_at)} · {detail.health.last_check.latency_ms} ms</small></span><span>{detail.health.last_check.error_message || "No provider error reported."}</span></div> : <EmptyState title="No health checks yet" body="Health checks begin after a provider adapter is configured." />}
                </Card>
                <Card>
                  <SectionHeader title="Credentials" description="References and status only; secret values are never returned" />
                  {detail.credentials.length ? detail.credentials.map((credential) => <div className="channel-event-row" key={credential.id}><span><strong>{humanStatus(credential.credential_type)}</strong><small>{credential.reference_type ? `${humanStatus(credential.reference_type)} reference` : "No reference"}</small></span><Badge tone={badgeTone(credential.status)}>{credential.status === "present" ? "Configured" : humanStatus(credential.status)}</Badge></div>) : <EmptyState title="No credential references" body="Add credentials in the provider setup panel." />}
                </Card>
              </div>

              <div hidden={tab !== "overview"}><Card id={selected.provider === "whatsapp" ? "whatsapp-activity" : undefined}>
                <SectionHeader title="Provider identity" description="Accounts and numbers attached to this connection" />
                {detail.provider_accounts.length === 0 && detail.identities.length === 0 ? <EmptyState title="No provider identity" body="An external provider account has not been authorized or provisioned." /> : <div className="channel-identity-list">
                  {detail.provider_accounts.map((account) => <div className="channel-event-row" key={account.id}><span><strong>{account.display_name || account.business_name || "Provider account"}</strong><small>Account</small></span><Badge tone={badgeTone(account.status)}>{humanStatus(account.status)}</Badge></div>)}
                  {detail.identities.map((identity) => <div className="channel-event-row" key={identity.id}><span><strong>{identity.identity_type === "phone_number" ? maskPhone(identity.display_name || identity.external_handle, "Connected number") : identity.display_name || identity.external_handle || "Channel identity"}</strong><small>{humanStatus(identity.identity_type)}</small></span><Badge tone={badgeTone(identity.status)}>{humanStatus(identity.status)}</Badge></div>)}
                </div>}
              </Card>

              </div><div hidden={tab !== "health"}><Card>
                <SectionHeader title="Provider events" description="Normalized setup, delivery, and provider activity" />
                {selected.provider === "whatsapp" && whatsApp ? (whatsApp.operational_events.length ? <div className="table-wrap"><table><thead><tr><th>Event</th><th>Status</th><th>Received</th><th>Next step</th></tr></thead><tbody>{whatsApp.operational_events.map((event, index) => <tr key={`${event.type}-${event.status}-${event.occurred_at}-${index}`}><td>{event.title}</td><td><Badge tone={badgeTone(event.status)}>{humanStatus(event.status)}</Badge></td><td>{relativeTime(event.occurred_at)}</td><td>{event.guidance || "No action required"}</td></tr>)}</tbody></table></div> : <EmptyState title="No WhatsApp activity" body="Verification, messages, and delivery updates will appear here." />) : events.length ? <div className="table-wrap"><table><thead><tr><th>Event</th><th>Status</th><th>Received</th><th>Note</th></tr></thead><tbody>{events.map((event) => <tr key={event.id}><td>{humanStatus(event.event_type)}</td><td><Badge tone={badgeTone(event.normalized_status)}>{humanStatus(event.normalized_status)}</Badge></td><td>{relativeTime(event.received_at)}</td><td>{event.processing_error || "—"}</td></tr>)}</tbody></table></div> : <EmptyState title="No provider events" body="Events will appear after an adapter receives normalized provider activity." />}
              </Card>

              </div><div hidden={tab !== "settings"}>{canManage ? <Card id={selected.provider === "whatsapp" ? "whatsapp-settings" : undefined}>
                <SectionHeader title="Connection settings" description="These settings do not authorize or connect an external provider." />
                <FormGrid onSubmit={saveConnection}>
                  <label>Display name<input value={editForm.display_name} onChange={(event) => setEditForm({ ...editForm, display_name: event.target.value })} required /></label>
                  <label>Managed by<select value={editForm.ownership_model} onChange={(event) => setEditForm({ ...editForm, ownership_model: event.target.value })}><option value="merchant_managed">You</option><option value="zidi_managed">Zidi</option></select></label>
                  <label>Environment<select value={editForm.environment} onChange={(event) => setEditForm({ ...editForm, environment: event.target.value })}><option value="sandbox">Sandbox</option><option value="production">Production</option></select></label>
                  <div className="full channel-settings-actions"><Button type="submit" icon={ShieldCheck} loading={saving} className="primary">Save settings</Button>{selected.status !== "archived" ? <ConfirmButton label="Archive connection" confirm="This removes the connection from active channel setup. Conversation and audit history remain available." onConfirm={() => void archiveConnection()} tone="danger" /> : null}</div>
                </FormGrid>
              </Card> : <p className="read-only-note">Channel configuration is read-only for your role.</p>}</div>
            </>}
          </div>
        </div>
      )}

      <Modal open={createOpen} title={`Connect ${providerLabel(createForm.provider)}`} description={createForm.provider === "whatsapp" ? "Name your connection, then choose Meta sign-in or manual credentials." : "Name your connection, then authorize with the provider. Workspace configuration and provider approval determine availability."} onClose={() => setCreateOpen(false)} footer={<><button type="button" className="ghost" onClick={() => setCreateOpen(false)}>Cancel</button><Button type="submit" form="channel-create-form" icon={Plus} className="primary" loading={saving}>Continue</Button></>}>
        <form id="channel-create-form" className="form-grid" onSubmit={createConnection}>
          <label>Display name<input value={createForm.display_name} onChange={(event) => setCreateForm({ ...createForm, display_name: event.target.value })} required /></label>
          <label>Managed by<select value={createForm.ownership_model} onChange={(event) => setCreateForm({ ...createForm, ownership_model: event.target.value })}><option value="merchant_managed">You</option><option value="zidi_managed">Zidi</option></select></label>
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
		  <div className="full"><WhatsAppPhone businessName="Business"><WhatsAppThread lines={[{ from: "bot", message: { type: "text", text: templatePreview() || "Template preview appears here." } }]} /></WhatsAppPhone></div>
		</form>
	  </Modal>
    </Page>
  );
}
