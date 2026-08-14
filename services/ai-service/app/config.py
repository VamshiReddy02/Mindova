"""Application settings, read from environment variables.

Deliberately minimal for now — just enough to configure which embedding
provider is active and its vector size. This grows the same way the Go
kernel's config package did: as real providers (OpenAI, Gemini, a local
model) get added, EMBEDDING_PROVIDER selects between them at the
composition root in app/api/embeddings.py.
"""

import os
from dataclasses import dataclass


@dataclass(frozen=True)
class Settings:
    embedding_provider: str = os.getenv("EMBEDDING_PROVIDER", "mock")
    embedding_dimensions: int = int(os.getenv("EMBEDDING_DIMENSIONS", "8"))


settings = Settings()