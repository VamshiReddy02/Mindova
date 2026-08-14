"""Deterministic mock embedding provider.

Mirrors the Go MockEmbedder in services/document-service/embedding/mock.go:
the same text always produces the same vector, different texts reliably
produce different vectors, and no external API is ever called. The values
carry no real semantic meaning — this exists purely so the rest of the
pipeline (chunking, storage, retrieval, and the tests around all of it)
can be exercised without depending on a real AI provider.
"""

import hashlib

from app.embedding.base import EmbeddingProvider

# Matches the document_chunks.embedding vector(8) column in PostgreSQL.
DEFAULT_DIMENSIONS = 8


class MockEmbeddingProvider(EmbeddingProvider):
    def __init__(self, dimensions: int = DEFAULT_DIMENSIONS) -> None:
        self.dimensions = dimensions if dimensions > 0 else DEFAULT_DIMENSIONS

    async def embed(self, texts: list[str]) -> list[list[float]]:
        return [self._fake_vector(text) for text in texts]

    def _fake_vector(self, text: str) -> list[float]:
        """Derives a deterministic vector from text via SHA-256, hashed
        once per dimension so each dimension varies independently. Values
        fall in the range [-1, 1), matching the shape (if not the
        semantics) of a typical real embedding.
        """
        vector: list[float] = []
        for dim in range(self.dimensions):
            digest = hashlib.sha256(f"{text}:{dim}".encode("utf-8")).digest()
            value = int.from_bytes(digest[:4], byteorder="big")
            vector.append((value % 2000) / 1000.0 - 1.0)
        return vector