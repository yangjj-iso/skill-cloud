"""HTTP client for the Skill Cloud platform."""
from __future__ import annotations

from dataclasses import dataclass
from typing import Any

import httpx


class SkillCloudError(RuntimeError):
    """Raised when the Skill Cloud API returns a non-2xx response."""

    def __init__(self, status_code: int, message: str) -> None:
        super().__init__(f"skill-cloud API error {status_code}: {message}")
        self.status_code = status_code
        self.message = message


@dataclass
class SkillSummary:
    """A summary view of a skill as returned by `GET /v1/skills`."""

    namespace: str
    name: str
    version: str
    description: str = ""

    @property
    def qualified_name(self) -> str:
        return f"{self.namespace}/{self.name}"

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> SkillSummary:
        return cls(
            namespace=data.get("namespace", ""),
            name=data.get("name", ""),
            version=data.get("version", ""),
            description=data.get("description", ""),
        )


class Client:
    """Synchronous HTTP client for Skill Cloud.

    Parameters
    ----------
    base_url:
        Base URL of the Skill Cloud server, e.g. ``"http://localhost:8080"``.
    api_key:
        API key used to authenticate requests. Sent as ``Authorization: Bearer``.
    timeout:
        Request timeout in seconds.
    """

    def __init__(
        self,
        base_url: str,
        api_key: str | None = None,
        timeout: float = 30.0,
    ) -> None:
        headers: dict[str, str] = {}
        if api_key:
            headers["Authorization"] = f"Bearer {api_key}"
        self._http = httpx.Client(
            base_url=base_url.rstrip("/"),
            headers=headers,
            timeout=timeout,
        )

    def close(self) -> None:
        self._http.close()

    def __enter__(self) -> Client:
        return self

    def __exit__(self, *exc_info: object) -> None:
        self.close()

    def list_skills(self) -> list[SkillSummary]:
        """List all skills the caller is authorized to see."""
        data = self._request("GET", "/v1/skills")
        return [SkillSummary.from_dict(s) for s in data.get("skills", [])]

    def get_skill(self, qualified_name: str) -> dict[str, Any]:
        """Return the full manifest for a skill given its ``namespace/name``."""
        ns, name = _split_qualified(qualified_name)
        return self._request("GET", f"/v1/skills/{ns}/{name}")

    def call(self, qualified_name: str, **inputs: Any) -> Any:
        """Invoke a skill synchronously and return its output."""
        ns, name = _split_qualified(qualified_name)
        return self._request(
            "POST",
            f"/v1/skills/{ns}/{name}/invoke",
            json=inputs,
        )

    def _request(
        self,
        method: str,
        path: str,
        *,
        json: dict[str, Any] | None = None,
    ) -> Any:
        resp = self._http.request(method, path, json=json)
        if resp.status_code >= 400:
            raise SkillCloudError(resp.status_code, resp.text)
        if resp.headers.get("content-type", "").startswith("application/json"):
            return resp.json()
        return resp.text


def _split_qualified(qualified_name: str) -> tuple[str, str]:
    if "/" not in qualified_name:
        raise ValueError(
            f"expected qualified skill name 'namespace/name', got {qualified_name!r}"
        )
    ns, _, name = qualified_name.partition("/")
    if not ns or not name:
        raise ValueError(
            f"expected qualified skill name 'namespace/name', got {qualified_name!r}"
        )
    return ns, name
