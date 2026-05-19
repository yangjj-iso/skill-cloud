# skill-cloud (Python SDK)

Python SDK for [Skill Cloud](https://github.com/yangjj-iso/skill-cloud) — a platform
that hosts remote skills callable by local agents.

```bash
pip install skill-cloud
```

```python
from skill_cloud import Client

client = Client(base_url="http://localhost:8080", api_key="...")

# Discover skills
for skill in client.list_skills():
    print(skill.qualified_name, skill.description)

# Invoke a skill
result = client.call("acme/hello", name="world")
print(result)  # -> {"message": "hello, world"}
```

See the main repo for more documentation and examples.
