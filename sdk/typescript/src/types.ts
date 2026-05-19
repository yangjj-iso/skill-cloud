export interface ClientOptions {
  /** Base URL of the Skill Cloud server, e.g. "http://localhost:8080". */
  baseUrl: string;
  /** API key sent as `Authorization: Bearer`. */
  apiKey?: string;
  /** Request timeout in milliseconds. Defaults to 30 000. */
  timeoutMs?: number;
  /** Override fetch (useful in tests). */
  fetch?: typeof fetch;
}

export interface SkillSummary {
  namespace: string;
  name: string;
  version: string;
  description?: string;
  /** Convenience: `${namespace}/${name}`. */
  qualifiedName: string;
}

export interface InvokeResult {
  status: string;
  skill?: string;
  input?: Record<string, unknown>;
  output?: unknown;
}
