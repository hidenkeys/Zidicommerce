import { FormEvent, useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { apiDelete, apiGet, apiPatch, apiPost } from "../api/client";
import { useAuth } from "../auth";
import { Badge, Card, ConfirmButton, EmptyState, FilterBar, Flash, FormGrid, LoadingState, Page, Tabs } from "../components/ui";
import { humanStatus, parseJSON, type Row } from "../lib/format";

type FAQ = Row & { id: string; question: string; answer: string; keywords?: string; status: string };

type KnowledgeEntry = Row & {
  id: string;
  kind: string;
  category: string;
  title: string;
  question: string;
  answer: string;
  keywords?: string;
  source_type: string;
  status: string;
  metadata?: string;
  embedding_status?: string;
  embedding_model?: string;
  embedded_at?: string;
  updated_at?: string;
};

type DocumentChunk = Row & {
  id: string;
  document_source_id: string;
  knowledge_entry_id?: string;
  chunk_index: number;
  title: string;
  heading: string;
  content: string;
  status: string;
};

type DocumentSource = Row & {
  id: string;
  title: string;
  source_type: string;
  status: string;
  source_label?: string;
  mime_type?: string;
  raw_text?: string;
  error_message?: string;
  chunks?: DocumentChunk[];
  linked_knowledge_count?: number;
  active_linked_knowledge_count?: number;
};

type KnowledgeForm = {
  kind: string;
  category: string;
  title: string;
  question: string;
  answer: string;
  keywords: string;
  status: string;
};

type DocumentForm = {
  title: string;
  source_type: string;
  source_label: string;
  raw_text: string;
};

type ChunkForm = {
  title: string;
  heading: string;
  content: string;
};

type ChunkApprovalForm = {
  kind: string;
  category: string;
  question: string;
  keywords: string;
};

const knowledgeKinds = [
  ["faq", "FAQ"],
  ["policy", "Policy"],
  ["business_info", "Business info"],
  ["delivery", "Delivery"],
  ["returns", "Returns"],
  ["warranty", "Warranty"],
  ["location", "Location"],
  ["payment_info", "Payment info"],
] as const;

const statuses = ["active", "draft", "archived"] as const;
const documentStatuses = ["review_required", "approved", "archived", "failed", "draft", "active", "processing", "extracted"] as const;

const blankForm: KnowledgeForm = {
  kind: "faq",
  category: "general",
  title: "",
  question: "",
  answer: "",
  keywords: "",
  status: "active",
};

const blankDocumentForm: DocumentForm = {
  title: "",
  source_type: "pasted_text",
  source_label: "",
  raw_text: "",
};

const blankChunkForm: ChunkForm = {
  title: "",
  heading: "",
  content: "",
};

const blankChunkApproval: ChunkApprovalForm = {
  kind: "policy",
  category: "policy",
  question: "",
  keywords: "",
};

export function KnowledgePage() {
  const { user } = useAuth();
  const canManageKnowledge = user.role === "platform_admin" || user.role === "merchant_admin";
  const [entries, setEntries] = useState<KnowledgeEntry[]>([]);
  const [faqs, setFaqs] = useState<FAQ[]>([]);
  const [documentSources, setDocumentSources] = useState<DocumentSource[]>([]);
  const [selectedSource, setSelectedSource] = useState<DocumentSource | null>(null);
  const [form, setForm] = useState<KnowledgeForm>(blankForm);
  const [documentForm, setDocumentForm] = useState<DocumentForm>(blankDocumentForm);
  const [documentFilters, setDocumentFilters] = useState({ status: "", source_type: "", search: "" });
  const [editingChunkID, setEditingChunkID] = useState("");
  const [chunkForm, setChunkForm] = useState<ChunkForm>(blankChunkForm);
  const [approvalForms, setApprovalForms] = useState<Record<string, ChunkApprovalForm>>({});
  const [editingID, setEditingID] = useState("");
  const [filters, setFilters] = useState({ kind: "", category: "", status: "", search: "" });
  const [query, setQuery] = useState("");
  const [match, setMatch] = useState("");
  const [message, setMessage] = useState("");
  const [flash, setFlash] = useState("");
  const [view, setView] = useState("entries");
  const [loading, setLoading] = useState(true);

  const categories = useMemo(() => {
    const values = new Set(entries.map((entry) => entry.category).filter(Boolean));
    return Array.from(values).sort();
  }, [entries]);

  async function load() {
    try {
      const params = new URLSearchParams();
      if (filters.kind) params.set("kind", filters.kind);
      if (filters.category) params.set("category", filters.category);
      if (filters.status) params.set("status", filters.status);
      if (filters.search) params.set("search", filters.search);
      const suffix = params.toString() ? `?${params.toString()}` : "";
      const docParams = new URLSearchParams();
      if (documentFilters.status) docParams.set("status", documentFilters.status);
      if (documentFilters.source_type) docParams.set("source_type", documentFilters.source_type);
      if (documentFilters.search) docParams.set("search", documentFilters.search);
      const docSuffix = docParams.toString() ? `?${docParams.toString()}` : "";
      const [knowledgeResponse, faqResponse, documentResponse] = await Promise.all([
        apiGet<KnowledgeEntry[]>(`/knowledge-entries${suffix}`),
        apiGet<FAQ[]>("/bot-faqs"),
        apiGet<DocumentSource[]>(`/document-sources${docSuffix}`),
      ]);
      setEntries(knowledgeResponse.data);
      setFaqs(faqResponse.data);
      setDocumentSources(documentResponse.data);
      setMessage("");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not load knowledge");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filters.kind, filters.category, filters.status, documentFilters.status, documentFilters.source_type]);

  async function loadDocumentSource(id: string) {
    const response = await apiGet<DocumentSource>(`/document-sources/${id}`);
    setSelectedSource(response.data);
    setEditingChunkID("");
    setChunkForm(blankChunkForm);
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    const body = {
      kind: form.kind,
      category: form.category,
      title: form.title,
      question: form.question,
      answer: form.answer,
      keywords: splitKeywords(form.keywords),
      status: form.status,
      source_type: "manual",
    };
    if (editingID) {
      await apiPatch<KnowledgeEntry>(`/knowledge-entries/${editingID}`, body);
      setFlash("Knowledge entry updated.");
    } else {
      await apiPost<KnowledgeEntry>("/knowledge-entries", body);
      setFlash("Knowledge entry added.");
    }
    setEditingID("");
    setForm(blankForm);
    await load();
  }

  function edit(entry: KnowledgeEntry) {
    setEditingID(entry.id);
    setForm({
      kind: entry.kind || "faq",
      category: entry.category || "general",
      title: entry.title || "",
      question: entry.question || "",
      answer: entry.answer || "",
      keywords: parseKeywords(entry.keywords).join(", "),
      status: entry.status || "active",
    });
    setFlash("");
  }

  async function archive(entry: KnowledgeEntry) {
    await apiDelete<KnowledgeEntry>(`/knowledge-entries/${entry.id}`);
    if (editingID === entry.id) {
      setEditingID("");
      setForm(blankForm);
    }
    setFlash("Knowledge entry archived.");
    await load();
  }

  async function applySearch() {
    await load();
  }

  async function testMatch() {
    try {
      const response = await apiPost<{ faq?: { answer?: string }; answer?: string }>("/bot-faqs/match", { query });
      setMatch(String(response.data.faq?.answer ?? response.data.answer ?? "Matched, but no answer text was returned."));
    } catch (error) {
      setMatch(error instanceof Error ? error.message : "No matching answer");
    }
  }

  async function createDocumentSource(event: FormEvent) {
    event.preventDefault();
    const response = await apiPost<DocumentSource>("/document-sources", documentForm);
    setDocumentForm(blankDocumentForm);
    setSelectedSource(response.data);
    setFlash("Document extracted into reviewable chunks.");
    await load();
  }

  async function archiveDocumentSource(source: DocumentSource, archiveLinkedKnowledge = false) {
    const suffix = archiveLinkedKnowledge ? "?archive_linked_knowledge=true" : "";
    const response = await apiDelete<DocumentSource>(`/document-sources/${source.id}${suffix}`);
    setSelectedSource(response.data);
    setFlash(archiveLinkedKnowledge ? "Document source and linked active knowledge archived." : "Document source archived. Linked active knowledge remains available to AI.");
    await load();
  }

  function editChunk(chunk: DocumentChunk) {
    setEditingChunkID(chunk.id);
    setChunkForm({ title: chunk.title || "", heading: chunk.heading || "", content: chunk.content || "" });
  }

  async function saveChunk(chunk: DocumentChunk) {
    await apiPatch<DocumentChunk>(`/document-chunks/${chunk.id}`, chunkForm);
    setEditingChunkID("");
    setChunkForm(blankChunkForm);
    setFlash("Document chunk updated.");
    await loadDocumentSource(chunk.document_source_id);
  }

  async function archiveChunk(chunk: DocumentChunk) {
    await apiDelete<DocumentChunk>(`/document-chunks/${chunk.id}`);
    setFlash("Document chunk archived.");
    await loadDocumentSource(chunk.document_source_id);
  }

  async function approveChunk(chunk: DocumentChunk) {
    const approval = approvalForms[chunk.id] ?? blankChunkApproval;
    await apiPost<DocumentChunk>(`/document-chunks/${chunk.id}/approve`, {
      kind: approval.kind,
      category: approval.category,
      question: approval.question,
      keywords: splitKeywords(approval.keywords),
    });
    setFlash("Document chunk approved into active structured knowledge.");
    await Promise.all([load(), loadDocumentSource(chunk.document_source_id)]);
  }

  const activeCount = entries.filter((entry) => entry.status === "active").length;
  const draftCount = entries.filter((entry) => entry.status === "draft").length;
  const archivedCount = entries.filter((entry) => entry.status === "archived").length;
  const reviewChunkCount = selectedSource?.chunks?.filter((chunk) => chunk.status === "review_required").length ?? 0;

  if (loading) {
    return <Page title="Business knowledge" description="Manage the facts, policies, and answers your assistant is allowed to use."><LoadingState label="Loading business knowledge" /></Page>;
  }

  return (
    <Page
      title="Business knowledge"
      description="Manage the facts, policies, and answers your assistant is allowed to use."
      help="Only active entries are used by AI grounding. Draft and archived entries stay available for review but are not used in answers."
    >
      <Flash message={message} />
      <Flash message={flash} tone="success" />

      <Tabs
        value={view}
        onChange={setView}
        label="Business knowledge sections"
        items={[
          { value: "entries", label: "Knowledge entries", count: entries.length },
          { value: "documents", label: "Documents", count: documentSources.length },
          { value: "legacy", label: "FAQ compatibility", count: faqs.filter((faq) => faq.status === "active").length },
        ]}
      />

      <div className="grid" style={{ marginBottom: 16 }}>
        <Card>
          <strong>Active</strong>
          <h3>{activeCount}</h3>
        </Card>
        <Card>
          <strong>Draft</strong>
          <h3>{draftCount}</h3>
        </Card>
        <Card>
          <strong>Archived</strong>
          <h3>{archivedCount}</h3>
        </Card>
        <Card>
          <strong>Legacy FAQs</strong>
          <h3>{faqs.filter((faq) => faq.status === "active").length}</h3>
        </Card>
        <Card>
          <strong>Documents</strong>
          <h3>{documentSources.filter((source) => source.status !== "archived").length}</h3>
        </Card>
      </div>

      {view === "entries" ? <>
      {canManageKnowledge ? (
        <FormGrid onSubmit={submit} title={editingID ? "Edit knowledge entry" : "Add knowledge entry"}>
          <label>
            Type
            <select value={form.kind} onChange={(event) => setForm({ ...form, kind: event.target.value })}>
              {knowledgeKinds.map(([value, label]) => <option key={value} value={value}>{label}</option>)}
            </select>
          </label>
          <label>
            Status
            <select value={form.status} onChange={(event) => setForm({ ...form, status: event.target.value })}>
              {statuses.map((status) => <option key={status} value={status}>{humanStatus(status)}</option>)}
            </select>
          </label>
          <label>
            Category
            <input value={form.category} onChange={(event) => setForm({ ...form, category: event.target.value })} placeholder="returns" />
          </label>
          <label>
            Keywords
            <input value={form.keywords} onChange={(event) => setForm({ ...form, keywords: event.target.value })} placeholder="refunds, exchange" />
          </label>
          <label className="full">
            Title
            <input value={form.title} onChange={(event) => setForm({ ...form, title: event.target.value })} placeholder="Returns policy" required />
          </label>
          <label className="full">
            Customer question
            <input value={form.question} onChange={(event) => setForm({ ...form, question: event.target.value })} placeholder="Can I return an item?" />
          </label>
          <label className="full">
            Answer
            <textarea value={form.answer} onChange={(event) => setForm({ ...form, answer: event.target.value })} placeholder="Returns are accepted within 7 days with receipt proof." required />
          </label>
          <div className="full filter-bar">
            <button type="submit">{editingID ? "Save changes" : "Add entry"}</button>
            {editingID ? <button type="button" className="ghost" onClick={() => { setEditingID(""); setForm(blankForm); }}>Cancel</button> : null}
          </div>
        </FormGrid>
      ) : (
        <Card>
          <strong>Read-only access</strong>
          <p className="muted">You can view merchant knowledge, but your role cannot create, edit, or archive entries.</p>
        </Card>
      )}

      <Card>
        <h3>Find knowledge</h3>
        <FilterBar>
          <input value={filters.search} onChange={(event) => setFilters({ ...filters, search: event.target.value })} placeholder="Search title, question, answer, keywords" />
          <select value={filters.kind} onChange={(event) => setFilters({ ...filters, kind: event.target.value })}>
            <option value="">All types</option>
            {knowledgeKinds.map(([value, label]) => <option key={value} value={value}>{label}</option>)}
          </select>
          <select value={filters.category} onChange={(event) => setFilters({ ...filters, category: event.target.value })}>
            <option value="">All categories</option>
            {categories.map((category) => <option key={category} value={category}>{humanStatus(category)}</option>)}
          </select>
          <select value={filters.status} onChange={(event) => setFilters({ ...filters, status: event.target.value })}>
            <option value="">All statuses</option>
            {statuses.map((status) => <option key={status} value={status}>{humanStatus(status)}</option>)}
          </select>
          <button type="button" onClick={() => void applySearch()}>Search</button>
        </FilterBar>
      </Card>

      <Card>
        {entries.length === 0 ? (
          <EmptyState title="No knowledge entries found" body="Add policies, FAQs, delivery details, warranty notes, and other merchant facts." />
        ) : entries.map((entry) => (
          <div key={entry.id} className={`setup-item ${entry.status === "active" ? "complete" : ""}`} style={{ marginBottom: 8, alignItems: "flex-start" }}>
            <span>
              <strong>{entry.title}</strong>
              <em>{entry.question || humanStatus(entry.kind)}</em>
              <em>{entry.answer}</em>
              <span className="filter-bar" style={{ marginTop: 8 }}>
                <Badge tone={entry.status === "active" ? "success" : entry.status === "draft" ? "warning" : "neutral"}>{humanStatus(entry.status)}</Badge>
                <Badge tone="info">{humanStatus(entry.kind)}</Badge>
                <Badge>{humanStatus(entry.category)}</Badge>
                {entry.embedding_status ? <Badge tone={embeddingBadgeTone(entry.embedding_status)}>{embeddingStatusLabel(entry.embedding_status)}</Badge> : null}
                {entry.source_type === "document_chunk" ? <Badge tone="info">Document sourced</Badge> : null}
              </span>
              {entry.source_type === "document_chunk" ? <em>{knowledgeTraceLabel(entry)}</em> : null}
            </span>
            {canManageKnowledge ? (
              <span className="filter-bar">
                <button type="button" className="ghost" onClick={() => edit(entry)}>Edit</button>
                {entry.status !== "archived" ? (
                  <ConfirmButton label="Archive" confirm="Archive this knowledge entry? Archived entries are not used by AI." onConfirm={() => void archive(entry)} tone="danger" />
                ) : null}
              </span>
            ) : null}
          </div>
        ))}
      </Card>
      </> : null}

      {view === "documents" ? <>
      <Card>
        <h3>Document ingestion</h3>
        <p className="muted">Pasted or manual documents are extracted into chunks for review. Chunks are not used by AI until approved into active structured knowledge.</p>
        {canManageKnowledge ? (
          <form onSubmit={createDocumentSource} className="form-grid" style={{ marginTop: 12 }}>
            <label>
              Source type
              <select value={documentForm.source_type} onChange={(event) => setDocumentForm({ ...documentForm, source_type: event.target.value })}>
                <option value="pasted_text">Pasted text</option>
                <option value="manual">Manual</option>
              </select>
            </label>
            <label>
              Source label
              <input value={documentForm.source_label} onChange={(event) => setDocumentForm({ ...documentForm, source_label: event.target.value })} placeholder="Staff handbook, returns PDF copy" />
            </label>
            <label className="full">
              Title
              <input value={documentForm.title} onChange={(event) => setDocumentForm({ ...documentForm, title: event.target.value })} placeholder="Delivery and returns policy" required />
            </label>
            <label className="full">
              Text
              <textarea value={documentForm.raw_text} onChange={(event) => setDocumentForm({ ...documentForm, raw_text: event.target.value })} placeholder="# Delivery&#10;Lagos delivery takes 24 to 48 hours." required />
            </label>
            <div className="full filter-bar">
              <button type="submit">Extract chunks</button>
            </div>
          </form>
        ) : null}

        <FilterBar>
          <input value={documentFilters.search} onChange={(event) => setDocumentFilters({ ...documentFilters, search: event.target.value })} placeholder="Search document title or label" />
          <select value={documentFilters.source_type} onChange={(event) => setDocumentFilters({ ...documentFilters, source_type: event.target.value })}>
            <option value="">All sources</option>
            <option value="pasted_text">Pasted text</option>
            <option value="manual">Manual</option>
          </select>
          <select value={documentFilters.status} onChange={(event) => setDocumentFilters({ ...documentFilters, status: event.target.value })}>
            <option value="">All statuses</option>
            {documentStatuses.map((status) => <option key={status} value={status}>{humanStatus(status)}</option>)}
          </select>
          <button type="button" onClick={() => void load()}>Search</button>
        </FilterBar>

        {documentSources.length === 0 ? (
          <EmptyState title="No document sources yet" body="Paste a policy, FAQ, warranty note, or business information document to extract reviewable chunks." />
        ) : documentSources.map((source) => (
          <div key={source.id} className={`setup-item ${source.status === "active" || source.status === "review_required" ? "complete" : ""}`} style={{ marginBottom: 8 }}>
            <span>
              <strong>{source.title}</strong>
              <em>{source.source_label || humanStatus(source.source_type)}</em>
              <span className="filter-bar" style={{ marginTop: 8 }}>
                <Badge tone={source.status === "archived" ? "neutral" : source.status === "failed" ? "danger" : "warning"}>{humanStatus(source.status)}</Badge>
                <Badge tone="info">{humanStatus(source.source_type)}</Badge>
                {(source.linked_knowledge_count ?? 0) > 0 ? <Badge tone="info">{source.linked_knowledge_count} linked knowledge</Badge> : null}
                {(source.active_linked_knowledge_count ?? 0) > 0 ? <Badge tone="success">{source.active_linked_knowledge_count} active</Badge> : null}
              </span>
            </span>
            <span className="filter-bar">
              <button type="button" className="ghost" onClick={() => void loadDocumentSource(source.id)}>View chunks</button>
              {canManageKnowledge && source.status !== "archived" ? (
                <>
                  <ConfirmButton label="Archive source" confirm="Archive this document source and its chunks? Linked active knowledge will remain available to AI." onConfirm={() => void archiveDocumentSource(source, false)} tone="danger" />
                  {(source.active_linked_knowledge_count ?? 0) > 0 ? (
                    <ConfirmButton label="Archive with knowledge" confirm="Archive this source, its chunks, and all linked active knowledge entries? This removes those approved facts from AI grounding." onConfirm={() => void archiveDocumentSource(source, true)} tone="danger" />
                  ) : null}
                </>
              ) : null}
            </span>
          </div>
        ))}
      </Card>

      {selectedSource ? (
        <Card>
          <h3>{selectedSource.title}</h3>
          <p className="muted">
            {reviewChunkCount} chunks require review. {selectedSource.linked_knowledge_count ?? 0} knowledge entries are linked to this source, including {selectedSource.active_linked_knowledge_count ?? 0} active entries.
          </p>
          <p className="muted">Approved chunks create active structured knowledge. Archiving only the source does not remove approved facts from AI grounding unless linked knowledge is archived deliberately.</p>
          {selectedSource.chunks?.length ? selectedSource.chunks.map((chunk) => {
            const approval = approvalForms[chunk.id] ?? blankChunkApproval;
            return (
              <div key={chunk.id} className={`setup-item ${chunk.status === "approved" ? "complete" : ""}`} style={{ marginBottom: 10, alignItems: "flex-start" }}>
                <span>
                  <strong>{chunk.title || `Chunk ${chunk.chunk_index + 1}`}</strong>
                  <em>{chunk.heading || `Chunk ${chunk.chunk_index + 1}`}</em>
                  {editingChunkID === chunk.id ? (
                    <span className="form-grid" style={{ marginTop: 8 }}>
                      <label>
                        Title
                        <input value={chunkForm.title} onChange={(event) => setChunkForm({ ...chunkForm, title: event.target.value })} />
                      </label>
                      <label>
                        Heading
                        <input value={chunkForm.heading} onChange={(event) => setChunkForm({ ...chunkForm, heading: event.target.value })} />
                      </label>
                      <label className="full">
                        Content
                        <textarea value={chunkForm.content} onChange={(event) => setChunkForm({ ...chunkForm, content: event.target.value })} />
                      </label>
                    </span>
                  ) : (
                    <em>{chunk.content}</em>
                  )}
                  <span className="filter-bar" style={{ marginTop: 8 }}>
                    <Badge tone={chunk.status === "approved" ? "success" : chunk.status === "archived" ? "neutral" : "warning"}>{humanStatus(chunk.status)}</Badge>
                    {chunk.knowledge_entry_id ? <Badge tone="success">Structured knowledge</Badge> : null}
                  </span>
                  {canManageKnowledge && chunk.status !== "archived" && chunk.status !== "approved" ? (
                    <span className="form-grid" style={{ marginTop: 10 }}>
                      <label>
                        Type
                        <select value={approval.kind} onChange={(event) => setApprovalForms({ ...approvalForms, [chunk.id]: { ...approval, kind: event.target.value, category: event.target.value } })}>
                          {knowledgeKinds.map(([value, label]) => <option key={value} value={value}>{label}</option>)}
                        </select>
                      </label>
                      <label>
                        Category
                        <input value={approval.category} onChange={(event) => setApprovalForms({ ...approvalForms, [chunk.id]: { ...approval, category: event.target.value } })} />
                      </label>
                      <label className="full">
                        Customer question
                        <input value={approval.question} onChange={(event) => setApprovalForms({ ...approvalForms, [chunk.id]: { ...approval, question: event.target.value } })} placeholder="What should customers ask to get this answer?" />
                      </label>
                      <label className="full">
                        Keywords
                        <input value={approval.keywords} onChange={(event) => setApprovalForms({ ...approvalForms, [chunk.id]: { ...approval, keywords: event.target.value } })} placeholder="delivery, returns, warranty" />
                      </label>
                    </span>
                  ) : null}
                </span>
                {canManageKnowledge ? (
                  <span className="filter-bar">
                    {editingChunkID === chunk.id ? (
                      <>
                        <button type="button" onClick={() => void saveChunk(chunk)}>Save</button>
                        <button type="button" className="ghost" onClick={() => { setEditingChunkID(""); setChunkForm(blankChunkForm); }}>Cancel</button>
                      </>
                    ) : (
                      <button type="button" className="ghost" onClick={() => editChunk(chunk)}>Edit</button>
                    )}
                    {chunk.status !== "approved" && chunk.status !== "archived" ? <button type="button" onClick={() => void approveChunk(chunk)}>Approve</button> : null}
                    {chunk.status !== "archived" ? <ConfirmButton label="Archive" confirm="Archive this chunk?" onConfirm={() => void archiveChunk(chunk)} tone="danger" /> : null}
                  </span>
                ) : null}
              </div>
            );
          }) : (
            <EmptyState title="No chunks extracted" body="Update the document source with reviewable text to generate chunks." />
          )}
        </Card>
      ) : null}

      </> : null}

      {view === "legacy" ? (
      <Card>
        <h3>Legacy FAQ matcher</h3>
        <p className="muted">This checks the existing FAQ matcher for backward compatibility. Structured knowledge is used by the AI grounding path.</p>
        <div className="filter-bar">
          <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Ask a sample FAQ question" />
          <button type="button" onClick={() => void testMatch()}>Test</button>
          <Link to="/assistant/ai-chat">Open AI chat</Link>
        </div>
        {match ? <p>{match}</p> : null}
      </Card>
      ) : null}
    </Page>
  );
}

function splitKeywords(value: string) {
  return value.split(",").map((keyword) => keyword.trim()).filter(Boolean);
}

function parseKeywords(value: string | undefined) {
  return parseJSON<string[]>(value, []);
}

function knowledgeTraceLabel(entry: KnowledgeEntry) {
  const trace = parseJSON<Record<string, string | number>>(entry.metadata, {});
  const sourceTitle = String(trace.document_source_title ?? "").trim();
  const sourceLabel = String(trace.document_source_label ?? "").trim();
  const approvedAt = String(trace.approved_at ?? "").trim();
  const source = sourceLabel || sourceTitle;
  const approved = approvedAt ? ` Approved ${new Date(approvedAt).toLocaleString()}.` : "";
  return source ? `From document: ${source}.${approved}` : `From approved document chunk.${approved}`;
}

function embeddingBadgeTone(status: string): "neutral" | "success" | "warning" | "danger" | "info" {
  switch (status) {
    case "ready":
      return "success";
    case "pending":
      return "warning";
    case "failed":
      return "danger";
    default:
      return "neutral";
  }
}

function embeddingStatusLabel(status: string) {
  switch (status) {
    case "ready":
      return "Ready for AI";
    case "pending":
      return "Preparing for AI";
    case "failed":
      return "AI preparation needs attention";
    case "disabled":
      return "Uses standard matching";
    default:
      return humanStatus(status);
  }
}
