import type { ApiDataResponse } from "@zidicommerce/shared";

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8080/v1";

type JsonBody = Record<string, unknown> | Array<unknown>;

export function getStoredToken() {
  return localStorage.getItem("zidicommerce_token") ?? "";
}

export function setStoredToken(token: string) {
  localStorage.setItem("zidicommerce_token", token.trim());
}

async function request<T>(method: string, path: string, body?: JsonBody): Promise<ApiDataResponse<T>> {
  const token = getStoredToken();
  const response = await fetch(`${API_BASE_URL}${path}`, {
    method,
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: body ? JSON.stringify(body) : undefined,
  });

  if (!response.ok) {
    const payload = await response.json().catch(() => null);
    const message = payload?.error?.message ?? `Request failed with status ${response.status}`;
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

export function apiDelete<T>(path: string) {
  return request<T>("DELETE", path);
}

export { API_BASE_URL };
