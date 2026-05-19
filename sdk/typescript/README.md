# @skill-cloud/client

TypeScript SDK for [Skill Cloud](https://github.com/yangjj-iso/skill-cloud) — a platform
that hosts remote skills callable by local agents.

```bash
npm install @skill-cloud/client
```

```ts
import { Client } from "@skill-cloud/client";

const client = new Client({
  baseUrl: "http://localhost:8080",
  apiKey: "...",
});

for (const skill of await client.listSkills()) {
  console.log(skill.qualifiedName, skill.description);
}

const result = await client.call("acme/hello", { name: "world" });
console.log(result);
```

See the main repo for more documentation and examples.
