/**
 * Thin fetch wrapper for REST API calls.
 *
 * Usage:
 *   const conversations = await api.get<ConversationSummary[]>('/api/conversations');
 *   const conv = await api.post<Conversation>('/api/conversations', { project_id: '...' });
 */

export class ApiError extends Error {
  status: number;
  statusText: string;
  body: string;

  constructor(status: number, statusText: string, body: string) {
    super(`API ${status}: ${statusText}`);
    this.name = "ApiError";
    this.status = status;
    this.statusText = statusText;
    this.body = body;
  }
}

async function request<T>(
  url: string,
  options?: RequestInit,
): Promise<T> {
  const res = await fetch(url, {
    headers: {
      "Content-Type": "application/json",
      ...options?.headers,
    },
    ...options,
  });

  if (!res.ok) {
    const body = await res.text();
    throw new ApiError(res.status, res.statusText, body);
  }

  // 204 No Content
  if (res.status === 204) {
    return undefined as T;
  }

  return res.json() as Promise<T>;
}

export const api = {
  get<T>(url: string): Promise<T> {
    return request<T>(url, { method: "GET" });
  },

  post<T>(url: string, body?: unknown): Promise<T> {
    return request<T>(url, {
      method: "POST",
      body: body ? JSON.stringify(body) : undefined,
    });
  },

  put<T>(url: string, body?: unknown): Promise<T> {
    return request<T>(url, {
      method: "PUT",
      body: body ? JSON.stringify(body) : undefined,
    });
  },

  delete<T>(url: string): Promise<T> {
    return request<T>(url, { method: "DELETE" });
  },
};
