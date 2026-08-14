"""Embedding provider abstraction.

Every concrete embedding source — a deterministic mock for tests, a real
provider like OpenAI or Gemini, or a locally-hosted model — implements
this same interface. The rest of the application (the FastAPI endpoint,
and later the Go Document Service calling into this one) depends only on
EmbeddingProvider, never on a specific implementation. Swapping providers
becomes a one-line change at the composition root, not a rewrite.
"""

from abc import ABC, abstractmethod


class EmbeddingProvider(ABC):
    """Generates vector embeddings for a batch of texts.

    The returned list has exactly one embedding per input text, in the
    same order as `texts`.
    """

    @abstractmethod
    async def embed(self, texts: list[str]) -> list[list[float]]:
        pass