import { describe, expect, it, beforeEach, vi, afterEach } from "vitest";
import {
  ApiError,
  getOverview,
  invokeSkill,
  listSkills,
  request,
  setApiKey,
} from "./api";

function fakeFetch(impl: (path: string, init?: RequestInit) => Promise<Response>) {
  globalThis.fetch = vi.fn(impl as unknown as typeof fetch);
}

beforeEach(() => {
  localStorage.clear();
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("api client", () => {
  it("attaches the API key from localStorage as a Bearer header", async () => {
    setApiKey("sc_live_key");
    fakeFetch(async (path, init) => {
      const headers = new Headers(init?.headers);
      expect(path).toBe("/v1/skills");
      expect(headers.get("Authorization")).toBe("Bearer sc_live_key");
      return new Response(JSON.stringify({ skills: [] }), { status: 200 });
    });
    const skills = await listSkills();
    expect(skills).toEqual([]);
  });

  it("throws an ApiError carrying the status code and body on non-2xx", async () => {
    setApiKey("sc_live_key");
    fakeFetch(async () => new Response("rate limit exceeded", { status: 429 }));
    await expect(request("/v1/skills")).rejects.toBeInstanceOf(ApiError);
    try {
      await request("/v1/skills");
    } catch (e) {
      expect((e as ApiError).status).toBe(429);
      expect((e as ApiError).body).toBe("rate limit exceeded");
    }
  });

  it("invokeSkill serialises the payload and parses the JSON reply", async () => {
    setApiKey("sc_live_key");
    fakeFetch(async (path, init) => {
      expect(path).toBe("/v1/skills/acme/hello/invoke");
      expect(init?.method).toBe("POST");
      expect(init?.body).toBe('{"name":"world"}');
      return new Response(JSON.stringify({ message: "hi" }), { status: 200 });
    });
    const out = await invokeSkill("acme", "hello", { name: "world" });
    expect(out).toEqual({ message: "hi" });
  });

  it("getOverview returns the parsed dashboard payload", async () => {
    setApiKey("sc_live_key");
    const overview = {
      skills_total: 2,
      skills_by_runtime: { docker: 2 },
      invocations_total: 17,
      invocations_24h: 5,
      recent: [],
    };
    fakeFetch(async () => new Response(JSON.stringify(overview), { status: 200 }));
    const got = await getOverview();
    expect(got.skills_total).toBe(2);
    expect(got.invocations_24h).toBe(5);
  });
});
