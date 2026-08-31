// A structured reason from a gated-error body — the app-wide deleteBlocker
// grammar (projects.go): the 409 PUT /api/global response carries
// {error, reasons: [{kind, target}, ...]} when live sessions block a root
// change (global.go). Kind is a stable machine token; Target is a
// human-readable subject (a tmux name or session label) rendered verbatim.
export interface ApiErrorReason {
  kind: string;
  target: string;
}

export class ApiError extends Error {
  status: number;
  // Populated only when the decoded error body carries a reasons array;
  // single-message error bodies leave it undefined — every existing
  // message-only caller behaves identically (16-01 additive widening).
  reasons?: ApiErrorReason[];

  constructor(message: string, status: number, reasons?: ApiErrorReason[]) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.reasons = reasons;
  }
}

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers);
  if (init?.body != null && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }

  const res = await fetch(path, { ...init, headers });

  if (!res.ok) {
    let message = res.statusText;
    let reasons: ApiErrorReason[] | undefined;
    try {
      const body = (await res.json()) as {
        error?: string;
        reasons?: ApiErrorReason[];
      };
      if (body.error) message = body.error;
      // The 409 gate bodies (PUT /api/global, project deletes) carry the
      // structured blocker list alongside the lead sentence — surface it.
      reasons = body.reasons;
    } catch {
      // non-JSON error body — keep statusText
    }
    throw new ApiError(message, res.status, reasons);
  }

  if (res.status === 204) {
    return undefined as T;
  }

  return (await res.json()) as T;
}

export function get<T>(path: string): Promise<T> {
  return api<T>(path);
}

export function post<T>(path: string, body?: unknown): Promise<T> {
  return api<T>(path, {
    method: "POST",
    body: body === undefined ? undefined : JSON.stringify(body),
  });
}

export function patch<T>(path: string, body?: unknown): Promise<T> {
  return api<T>(path, {
    method: "PATCH",
    body: body === undefined ? undefined : JSON.stringify(body),
  });
}

export function put<T>(path: string, body?: unknown): Promise<T> {
  return api<T>(path, {
    method: "PUT",
    body: body === undefined ? undefined : JSON.stringify(body),
  });
}

export function del<T = void>(path: string, body?: unknown): Promise<T> {
  return api<T>(path, {
    method: "DELETE",
    body: body === undefined ? undefined : JSON.stringify(body),
  });
}
