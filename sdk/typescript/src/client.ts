import { SkillCloudError } from "./errors.js";
import type { ClientOptions, InvokeResult, SkillSummary } from "./types.js";

/** HTTP client for the Skill Cloud platform. */
export class Client {
  private readonly baseUrl: string;
  private readonly apiKey?: string;
  private readonly timeoutMs: number;
  private readonly fetchImpl: typeof fetch;

  constructor(options: ClientOptions) {
    this.baseUrl = options.baseUrl.replace(/\/+$/, "");
    this.apiKey = options.apiKey;
    this.timeoutMs = options.timeoutMs ?? 30_000;
    this.fetchImpl = options.fetch ?? fetch;
  }

  /** List every skill the caller is authorized to see. */
  async listSkills(): Promise<SkillSummary[]> {
    const data = await this.request<{ skills?: Array<Record<string, unknown>> }>(
      "GET",
      "/v1/skills",
    );
    return (data.skills ?? []).map((s) => normalizeSkill(s));
  }

  /** Return the full manifest for `<namespace>/<name>`. */
  async getSkill(qualifiedName: string): Promise<Record<string, unknown>> {
    const [ns, name] = splitQualified(qualifiedName);
    return this.request<Record<string, unknown>>("GET", `/v1/skills/${ns}/${name}`);
  }

  /** Invoke a skill synchronously. */
  async call(
    qualifiedName: string,
    inputs: Record<string, unknown> = {},
  ): Promise<InvokeResult> {
    const [ns, name] = splitQualified(qualifiedName);
    return this.request<InvokeResult>(
      "POST",
      `/v1/skills/${ns}/${name}/invoke`,
      inputs,
    );
  }

  private async request<T>(method: string, path: string, body?: unknown): Promise<T> {
    const headers: Record<string, string> = {};
    if (this.apiKey) headers["Authorization"] = `Bearer ${this.apiKey}`;
    if (body !== undefined) headers["Content-Type"] = "application/json";

    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), this.timeoutMs);
    try {
      const resp = await this.fetchImpl(`${this.baseUrl}${path}`, {
        method,
        headers,
        body: body === undefined ? undefined : JSON.stringify(body),
        signal: controller.signal,
      });
      const text = await resp.text();
      if (!resp.ok) {
        throw new SkillCloudError(resp.status, text);
      }
      if (!text) return undefined as T;
      const contentType = resp.headers.get("content-type") ?? "";
      if (contentType.includes("application/json")) {
        return JSON.parse(text) as T;
      }
      return text as unknown as T;
    } finally {
      clearTimeout(timer);
    }
  }
}

function splitQualified(qualifiedName: string): [string, string] {
  const idx = qualifiedName.indexOf("/");
  if (idx <= 0 || idx === qualifiedName.length - 1) {
    throw new Error(
      `expected qualified skill name 'namespace/name', got '${qualifiedName}'`,
    );
  }
  return [qualifiedName.slice(0, idx), qualifiedName.slice(idx + 1)];
}

function normalizeSkill(s: Record<string, unknown>): SkillSummary {
  const namespace = String(s.namespace ?? "");
  const name = String(s.name ?? "");
  return {
    namespace,
    name,
    version: String(s.version ?? ""),
    description: s.description === undefined ? undefined : String(s.description),
    qualifiedName: `${namespace}/${name}`,
  };
}
