// Tiny fetch wrapper for the Skill Cloud REST API. The base URL is
// always relative ("/v1/...") because the Vite dev server proxies and
// the production build is served behind the same origin as the API.

export type Runtime = {
  type: "docker" | "http_proxy";
  // Other fields are only available via the dedicated runtime endpoint;
  // see /v1/skills/:ns/:name/runtime.
};

export type Skill = {
  namespace: string;
  name: string;
  version: string;
  description?: string;
  tags?: string[];
  runtime: Runtime;
};

export type Invocation = {
  status: string;
  latency_ms: number;
  input_bytes: number;
  output_bytes: number;
  started_at: string;
  version: string;
  caller_ip?: string;
  user_agent?: string;
  error_message?: string;
  // The /v1/invocations endpoint fills these so a single table can list
  // calls across skills.
  namespace?: string;
  name?: string;
};

export type Stats = {
  total: number;
  last_24h: number;
  last_invoked_at?: string;
  last_caller_ip?: string;
};

export type Overview = {
  skills_total: number;
  skills_by_runtime: Record<string, number>;
  invocations_24h: number;
  invocations_total: number;
  recent: Invocation[];
};

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly body: string,
  ) {
    super(`HTTP ${status}: ${body}`);
  }
}

function getApiKey(): string {
  return localStorage.getItem("skillcloud.apiKey") ?? "";
}

export function setApiKey(key: string): void {
  if (key) {
    localStorage.setItem("skillcloud.apiKey", key);
  } else {
    localStorage.removeItem("skillcloud.apiKey");
  }
}

export async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers ?? {});
  if (!headers.has("Authorization")) {
    const key = getApiKey();
    if (key) {
      headers.set("Authorization", `Bearer ${key}`);
    }
  }
  if (init.body && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  const res = await fetch(path, { ...init, headers });
  const text = await res.text();
  if (!res.ok) {
    throw new ApiError(res.status, text);
  }
  return text ? (JSON.parse(text) as T) : ({} as T);
}

export async function listSkills(): Promise<Skill[]> {
  const { skills } = await request<{ skills: Skill[] }>("/v1/skills");
  return skills;
}

export async function getSkill(ns: string, name: string): Promise<Skill> {
  return request<Skill>(`/v1/skills/${ns}/${name}`);
}

export async function getStats(ns: string, name: string): Promise<Stats> {
  return request<Stats>(`/v1/skills/${ns}/${name}/stats`);
}

export async function listSkillLogs(
  ns: string,
  name: string,
  limit = 50,
): Promise<Invocation[]> {
  const { invocations } = await request<{ invocations: Invocation[] }>(
    `/v1/skills/${ns}/${name}/logs?limit=${limit}`,
  );
  return invocations;
}

export async function getOverview(): Promise<Overview> {
  return request<Overview>("/v1/overview");
}

export async function listInvocations(limit = 100): Promise<Invocation[]> {
  const { invocations } = await request<{ invocations: Invocation[] }>(
    `/v1/invocations?limit=${limit}`,
  );
  return invocations;
}

export async function invokeSkill(
  ns: string,
  name: string,
  input: unknown,
): Promise<unknown> {
  return request(`/v1/skills/${ns}/${name}/invoke`, {
    method: "POST",
    body: JSON.stringify(input ?? {}),
  });
}
