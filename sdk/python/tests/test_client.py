from __future__ import annotations

import httpx
import pytest
from pytest_httpx import HTTPXMock

from skill_cloud import Client, SkillCloudError


def test_list_skills(httpx_mock: HTTPXMock) -> None:
    httpx_mock.add_response(
        method="GET",
        url="http://example.com/v1/skills",
        json={
            "skills": [
                {
                    "namespace": "acme",
                    "name": "hello",
                    "version": "0.1.0",
                    "description": "Say hello.",
                }
            ]
        },
    )

    with Client(base_url="http://example.com", api_key="k") as client:
        skills = client.list_skills()

    assert len(skills) == 1
    assert skills[0].qualified_name == "acme/hello"
    assert skills[0].description == "Say hello."


def test_call_skill(httpx_mock: HTTPXMock) -> None:
    httpx_mock.add_response(
        method="POST",
        url="http://example.com/v1/skills/acme/hello/invoke",
        json={"output": {"message": "hello, world"}, "status": "ok"},
    )

    with Client(base_url="http://example.com", api_key="k") as client:
        result = client.call("acme/hello", name="world")

    assert result["status"] == "ok"
    assert result["output"]["message"] == "hello, world"


def test_error_response(httpx_mock: HTTPXMock) -> None:
    httpx_mock.add_response(
        method="GET",
        url="http://example.com/v1/skills/acme/missing",
        status_code=404,
        text="not found",
    )

    with Client(base_url="http://example.com") as client, pytest.raises(SkillCloudError) as ei:
        client.get_skill("acme/missing")

    assert ei.value.status_code == 404


def test_qualified_name_must_be_valid() -> None:
    with Client(base_url="http://example.com") as client:
        with pytest.raises(ValueError):
            client.call("bad-name")


def test_auth_header_is_sent(httpx_mock: HTTPXMock) -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        assert request.headers.get("Authorization") == "Bearer secret"
        return httpx.Response(200, json={"skills": []})

    httpx_mock.add_callback(handler, method="GET", url="http://example.com/v1/skills")

    with Client(base_url="http://example.com", api_key="secret") as client:
        client.list_skills()
