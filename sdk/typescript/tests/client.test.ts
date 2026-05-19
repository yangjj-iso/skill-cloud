import { describe, expect, it } from "vitest";
import { Client, SkillCloudError } from "../src/index.js";

function mockFetch(handler: (input: Request) => Response | Promise<Response>): typeof fetch {
  return ((input: RequestInfo | URL, init?: RequestInit) => {
    const req = new Request(input as RequestInfo, init);
    return Promise.resolve(handler(req));
  }) as typeof fetch;
}

describe("Client", () => {
  it("lists skills", async () => {
    const client = new Client({
      baseUrl: "http://example.com",
      fetch: mockFetch(async (req) => {
        expect(new URL(req.url).pathname).toBe("/v1/skills");
        return new Response(
          JSON.stringify({
            skills: [
              { namespace: "acme", name: "hello", version: "0.1.0", description: "hi" },
            ],
          }),
          { status: 200, headers: { "content-type": "application/json" } },
        );
      }),
    });

    const skills = await client.listSkills();
    expect(skills).toHaveLength(1);
    expect(skills[0].qualifiedName).toBe("acme/hello");
  });

  it("calls a skill", async () => {
    const client = new Client({
      baseUrl: "http://example.com",
      apiKey: "secret",
      fetch: mockFetch(async (req) => {
        expect(req.method).toBe("POST");
        expect(req.headers.get("authorization")).toBe("Bearer secret");
        const body = await req.json();
        expect(body).toEqual({ name: "world" });
        return new Response(
          JSON.stringify({ status: "ok", output: { message: "hello, world" } }),
          { status: 200, headers: { "content-type": "application/json" } },
        );
      }),
    });

    const result = await client.call("acme/hello", { name: "world" });
    expect(result.status).toBe("ok");
  });

  it("raises SkillCloudError on non-2xx", async () => {
    const client = new Client({
      baseUrl: "http://example.com",
      fetch: mockFetch(async () => new Response("not found", { status: 404 })),
    });
    await expect(client.getSkill("acme/missing")).rejects.toBeInstanceOf(SkillCloudError);
  });

  it("rejects malformed qualified names", async () => {
    const client = new Client({ baseUrl: "http://example.com" });
    await expect(client.call("bad-name")).rejects.toThrow(/qualified skill name/);
  });
});
