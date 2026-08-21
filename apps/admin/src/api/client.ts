import type { ApiDataResponse } from "@zidicommerce/shared";

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8080/v1";

type JsonBody = Record<string, unknown> | Array<unknown>;

const TOKEN_KEY = "zidicommerce_token";
const PUBLIC_PATHS = new Set(["/auth/login", "/auth/register"]);

export const UNAUTHORIZED_EVENT = "zidicommerce:unauthorized";

export function getStoredToken() {
  return localStorage.getItem(TOKEN_KEY) ?? "";
}

export function setStoredToken(token: string) {
  localStorage.setItem(TOKEN_KEY, token.trim());
}

export function clearStoredToken() {
  localStorage.removeItem(TOKEN_KEY);
}

async function request<T>(method: string, path: string, body?: JsonBody): Promise<ApiDataResponse<T>> {
  const token = getStoredToken();
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
  };
  if (token && !PUBLIC_PATHS.has(path)) {
    headers.Authorization = `Bearer ${token}`;
  }

  let response: Response;
  try {
    response = await fetch(`${API_BASE_URL}${path}`, {
      method,
      headers,
      body: body ? JSON.stringify(body) : undefined,
    });
  } catch {
    throw new Error("Network error. Could not reach the API.");
  }

  if (!response.ok) {
    const payload = await response.json().catch(() => null);
    const message = payload?.error?.message ?? `Request failed with status ${response.status}`;
    if (response.status === 401 && !PUBLIC_PATHS.has(path)) {
      clearStoredToken();
      window.dispatchEvent(new Event(UNAUTHORIZED_EVENT));
    }
    throw new Error(message);
  }

  return response.json() as Promise<ApiDataResponse<T>>;
}

export function apiGet<T>(path: string) {
  return request<T>("GET", path);
}

export function apiPost<T>(path: string, body: JsonBody) {
  return request<T>("POST", path, body);
}

export function apiPatch<T>(path: string, body: JsonBody) {
  return request<T>("PATCH", path, body);
}

export function apiPut<T>(path: string, body: JsonBody) {
  return request<T>("PUT", path, body);
}

export function apiDelete<T>(path: string) {
  return request<T>("DELETE", path);
}

export { API_BASE_URL };
