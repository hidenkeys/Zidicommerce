const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8080/v1";
const TOKEN_KEY = "zidicommerce_field_token";

export function getToken() {
  return localStorage.getItem(TOKEN_KEY) ?? "";
}
export function setToken(token: string) {
  localStorage.setItem(TOKEN_KEY, token);
}
export function clearToken() {
  localStorage.removeItem(TOKEN_KEY);
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  headers.set("Content-Type", "application/json");
  const token = getToken();
  if (token) headers.set("Authorization", `Bearer ${token}`);
  const response = await fetch(`${API_BASE_URL}${path}`, { ...init, headers });
  const body = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(body.error?.message || body.message || "Request failed");
  }
  return (body.data ?? body) as T;
}

export const api = {
  login: async (email: string, password: string) => {
    const result = await request<Record<string, unknown>>("/auth/login", { method: "POST", body: JSON.stringify({ email, password }) });
    const nested = result.data as { access_token?: string } | undefined;
    return { access_token: String(result.access_token || nested?.access_token || "") };
  },
  me: () => request<{ id: string; email?: string; role: string; organization_id: string }>("/auth/me"),
  overview: () => request<Record<string, number>>("/field/overview"),
  settings: () => request<Record<string, unknown>>("/field/settings"),
  saveSettings: (body: Record<string, unknown>) => request("/field/settings", { method: "PUT", body: JSON.stringify(body) }),
  pools: () => request<Array<Record<string, unknown>>>("/field/pools"),
  providers: () => request<Array<Record<string, unknown>>>("/field/providers"),
  saveProvider: (body: Record<string, unknown>) => request<Record<string, unknown>>("/field/providers", { method: "POST", body: JSON.stringify(body) }),
  savePool: (body: Record<string, unknown>) => request<Record<string, unknown>>("/field/pools", { method: "POST", body: JSON.stringify(body) }),
  requests: (status = "") => request<Array<Record<string, unknown>>>(`/field/requests${status ? `?status=${status}` : ""}`),
  request: (id: string) => request<Record<string, unknown>>(`/field/requests/${id}`),
  matches: (id: string) => request<Array<Record<string, unknown>>>(`/field/requests/${id}/matches`),
  messages: (id: string) => request<Array<Record<string, unknown>>>(`/field/requests/${id}/messages`),
  sendMessage: (id: string, body: string) => request(`/field/requests/${id}/messages`, { method: "POST", body: JSON.stringify({ body }) }),
  quotes: () => request<Array<Record<string, unknown>>>("/field/quotes"),
  home: () => request<Record<string, unknown>>("/field/home"),
  inbox: () => request<Array<Record<string, unknown>>>("/field/inbox"),
  accept: (id: string) => request(`/field/dispatch/${id}/accept`, { method: "POST", body: "{}" }),
  decline: (id: string) => request(`/field/dispatch/${id}/decline`, { method: "POST", body: "{}" }),
  status: (id: string, status: string) => request(`/field/requests/${id}/status`, { method: "POST", body: JSON.stringify({ status }) }),
  quote: (id: string, body: Record<string, unknown>) => request(`/field/requests/${id}/quotes`, { method: "POST", body: JSON.stringify(body) }),
  takeOver: (id: string) => request(`/field/requests/${id}/take-over`, { method: "POST", body: "{}" }),
  release: (id: string) => request(`/field/requests/${id}/release`, { method: "POST", body: "{}" }),
  close: (id: string) => request(`/field/requests/${id}/close`, { method: "POST", body: "{}" }),
  customers: () => request<Array<Record<string, unknown>>>("/customers"),
  audit: () => request<Array<Record<string, unknown>>>("/organizations/current/audit-logs"),
  availability: (id: string, availability: string) => request(`/field/providers/${id}/availability`, { method: "POST", body: JSON.stringify({ availability }) }),
  conversations: () => request<Array<Record<string, unknown>>>("/field/conversations"),
  transactions: () => request<Array<Record<string, unknown>>>("/field/transactions"),
};
