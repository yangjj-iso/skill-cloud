# http-proxy-skill

Demonstrates the `http_proxy` runtime: instead of uploading code, this skill
registers an external HTTPS endpoint. When the skill is invoked, Skill Cloud
forwards the inputs to that endpoint and returns its response.

This is the right choice when:

- The skill is already running as its own service.
- You don't want to give Skill Cloud your code.
- The skill needs to live in your own VPC.
