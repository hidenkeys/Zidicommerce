import type { ApiDataResponse } from "@zidicommerce/shared";

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8080/v1";

export async function apiGet<T>(path: string, token?: string): Promise<ApiDataResponse<T>> {
  const response = await fetch(`${API_BASE_URL}${path}`, {
    headers: token ? { Authorization: `Bearer ${token}` } : undefined,
  });

  if (!response.ok) {
    throw new Error(`Request failed with status ${response.status}`);
  }

  return response.json() as Promise<ApiDataResponse<T>>;
}

export { API_BASE_URL };

