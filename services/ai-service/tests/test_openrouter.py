"""Tests for OpenRouterProvider.

Every test injects an httpx.AsyncClient wired to httpx.MockTransport, so
no real network call is ever made and no OpenRouter quota is consumed.
Async calls are driven via asyncio.run() inside plain sync test
functions, avoiding a pytest-asyncio dependency for a handful of tests.
"""

import asyncio
import json

import httpx

from app.llm.openrouter import OpenRouterError, OpenRouterProvider


def _make_client(handler) -> httpx.AsyncClient:
    return httpx.AsyncClient(transport=httpx.MockTransport(handler))


def test_openrouter_success():
    captured: dict = {}

    def handler(request: httpx.Request) -> httpx.Response:
        captured["auth"] = request.headers.get("authorization")
        captured["content_type"] = request.headers.get("content-type")
        captured["body"] = request.read()
        return httpx.Response(
            200,
            json={
                "choices": [
                    {"message": {"role": "assistant", "content": "the generated answer"}}
                ]
            },
        )

    client = _make_client(handler)
    provider = OpenRouterProvider(api_key="test-key", model="openrouter/free", client=client)

    answer = asyncio.run(
        provider.generate([{"role": "user", "content": "Explain Mindova in one sentence."}])
    )
    asyncio.run(client.aclose())

    assert answer == "the generated answer"
    assert captured["auth"] == "Bearer test-key"
    assert captured["content_type"] == "application/json"

    sent_body = json.loads(captured["body"])
    assert sent_body["model"] == "openrouter/free"
    assert sent_body["messages"] == [
        {"role": "user", "content": "Explain Mindova in one sentence."}
    ]


def test_openrouter_empty_messages():
    request_made = False

    def handler(request: httpx.Request) -> httpx.Response:
        nonlocal request_made
        request_made = True
        return httpx.Response(200, json={"choices": [{"message": {"content": "x"}}]})

    client = _make_client(handler)
    provider = OpenRouterProvider(api_key="test-key", client=client)

    try:
        asyncio.run(provider.generate([]))
        assert False, "expected OpenRouterError"
    except OpenRouterError:
        pass
    finally:
        asyncio.run(client.aclose())

    assert request_made is False, "expected no HTTP request for empty messages"


def test_openrouter_connection_failure():
    def handler(request: httpx.Request) -> httpx.Response:
        raise httpx.ConnectError("connection refused", request=request)

    client = _make_client(handler)
    provider = OpenRouterProvider(api_key="test-key", client=client)

    try:
        asyncio.run(provider.generate([{"role": "user", "content": "hello"}]))
        assert False, "expected OpenRouterError"
    except OpenRouterError:
        pass
    finally:
        asyncio.run(client.aclose())


def test_openrouter_non_success_status():
    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(500, text='{"error":"upstream unavailable"}')

    client = _make_client(handler)
    provider = OpenRouterProvider(api_key="test-key", client=client)

    try:
        asyncio.run(provider.generate([{"role": "user", "content": "hello"}]))
        assert False, "expected OpenRouterError"
    except OpenRouterError:
        pass
    finally:
        asyncio.run(client.aclose())


def test_openrouter_invalid_json():
    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(200, text="not valid json{{{")

    client = _make_client(handler)
    provider = OpenRouterProvider(api_key="test-key", client=client)

    try:
        asyncio.run(provider.generate([{"role": "user", "content": "hello"}]))
        assert False, "expected OpenRouterError"
    except OpenRouterError:
        pass
    finally:
        asyncio.run(client.aclose())


def test_openrouter_empty_answer():
    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(
            200,
            json={"choices": [{"message": {"role": "assistant", "content": ""}}]},
        )

    client = _make_client(handler)
    provider = OpenRouterProvider(api_key="test-key", client=client)

    try:
        asyncio.run(provider.generate([{"role": "user", "content": "hello"}]))
        assert False, "expected OpenRouterError"
    except OpenRouterError:
        pass
    finally:
        asyncio.run(client.aclose())