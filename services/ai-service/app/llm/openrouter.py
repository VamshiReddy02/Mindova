"""OpenRouter LLM provider.

Calls OpenRouter's OpenAI-compatible chat completions endpoint. OpenRouter
proxies many models behind one API; "openrouter/free" auto-routes to
whichever free model is currently available, which is what makes this a
genuinely usable real provider without a paid API key.

This is the first LLMProvider implementation that leaves the process —
everything else in app/llm/ so far has been local and deterministic.
"""

import httpx

from app.llm.base import LLMProvider

OPENROUTER_URL = "https://openrouter.ai/api/v1/chat/completions"
DEFAULT_TIMEOUT = 30.0


class OpenRouterError(Exception):
    """Raised for any failure calling OpenRouter: connection failure, a
    non-2xx response, invalid JSON, an unexpected response shape, or an
    empty answer. Callers (the /v1/chat/completions endpoint) can catch
    this specifically to distinguish "OpenRouter failed" from other bugs.
    """


class OpenRouterProvider(LLMProvider):
    def __init__(
        self,
        api_key: str,
        model: str = "openrouter/free",
        client: httpx.AsyncClient | None = None,
        timeout: float = DEFAULT_TIMEOUT,
    ) -> None:
        self.api_key = api_key
        self.model = model
        self._client = client
        self._timeout = timeout

    async def generate(self, messages: list[dict[str, str]]) -> str:
        if not messages:
            raise OpenRouterError("no messages provided")

        payload = {"model": self.model, "messages": messages}
        headers = {
            "Authorization": f"Bearer {self.api_key}",
            "Content-Type": "application/json",
        }

        # Tests inject their own client (wired to httpx.MockTransport, no
        # real network call); real usage creates a fresh client per call
        # and closes it, rather than holding one open for the process
        # lifetime.
        client = self._client
        owns_client = client is None
        if owns_client:
            client = httpx.AsyncClient(timeout=self._timeout)

        try:
            response = await client.post(OPENROUTER_URL, json=payload, headers=headers)
        except httpx.HTTPError as exc:
            raise OpenRouterError(f"request to OpenRouter failed: {exc}") from exc
        finally:
            if owns_client:
                await client.aclose()

        if response.status_code < 200 or response.status_code >= 300:
            raise OpenRouterError(
                f"OpenRouter returned status {response.status_code}: {response.text.strip()}"
            )

        try:
            data = response.json()
        except ValueError as exc:
            raise OpenRouterError(f"failed to parse OpenRouter response JSON: {exc}") from exc

        try:
            content = data["choices"][0]["message"]["content"]
        except (KeyError, IndexError, TypeError) as exc:
            raise OpenRouterError(f"unexpected OpenRouter response shape: {exc}") from exc

        if not content:
            raise OpenRouterError("OpenRouter returned an empty answer")

        return content