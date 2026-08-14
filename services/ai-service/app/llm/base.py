"""LLM provider abstraction.

Mirrors app/embedding/base.py: every concrete answer-generation source —
a deterministic mock for tests, or a real provider like OpenAI or
Gemini — implements this same interface. The rest of the application (the
chat completion endpoint, and eventually the Go retrieval pipeline calling
into it) depends only on LLMProvider, never a specific implementation.

messages are plain dicts, not the ChatMessage pydantic model, so this
interface has zero dependency on the API layer's request/response
schemas. That matters because real providers' SDKs (OpenAI's included)
already expect messages as list[dict[str, str]] — this keeps the
provider boundary a direct match for what a real implementation will
receive and pass straight through, with no schema translation needed.
"""

from abc import ABC, abstractmethod


class LLMProvider(ABC):
    """Generates a chat completion from a sequence of messages.

    Each message is a dict with "role" and "content" keys, following the
    same shape most chat APIs use ("system", "user", "assistant").
    """

    @abstractmethod
    async def generate(
        self,
        messages: list[dict[str, str]],
    ) -> str:
        pass