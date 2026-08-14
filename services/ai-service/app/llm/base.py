"""LLM provider abstraction.

Mirrors app/embedding/base.py: every concrete answer-generation source —
a deterministic mock for tests, or a real provider like OpenAI or
Gemini — implements this same interface. The rest of the application (the
chat completion endpoint, and eventually the Go retrieval pipeline calling
into it) depends only on LLMProvider, never a specific implementation.
"""

from abc import ABC, abstractmethod

from app.schemas.chat import ChatMessage


class LLMProvider(ABC):
    """Generates a chat completion from a sequence of messages.

    messages follows the same role-based shape most chat APIs use
    ("system", "user", "assistant"), so a real provider swapped in later
    can consume the same input with little to no translation.
    """

    @abstractmethod
    async def complete(self, messages: list[ChatMessage]) -> str:
        pass