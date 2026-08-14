"""Deterministic mock LLM provider.

Mirrors app/embedding/mock.py in spirit: no external API is ever called,
and the same input always produces the same output. This exists purely so
the RAG pipeline's plumbing — retrieval, context assembly, the HTTP call
into this service, and the response shape — can be built and tested
end-to-end before paying for (or depending on the availability of) a real
LLM provider.

The output carries no real reasoning or intelligence: it's a template
that reflects back what it was given, so tests can assert on structure
(the question made it through, the context made it through) without
depending on unpredictable model output.
"""

from app.llm.base import LLMProvider
from app.schemas.chat import ChatMessage


class MockLLMProvider(LLMProvider):
    async def complete(self, messages: list[ChatMessage]) -> str:
        question = self._last_user_message(messages)
        context = self._first_system_message(messages)

        if context:
            return (
                f"[mock answer] Based on the provided context, "
                f"here is a response to: {question}"
            )
        return f"[mock answer] {question}"

    @staticmethod
    def _last_user_message(messages: list[ChatMessage]) -> str:
        for message in reversed(messages):
            if message.role == "user":
                return message.content
        return ""

    @staticmethod
    def _first_system_message(messages: list[ChatMessage]) -> str:
        for message in messages:
            if message.role == "system":
                return message.content
        return ""