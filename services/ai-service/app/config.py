"""Application settings, read from environment variables.

Deliberately minimal for now — just enough to configure which embedding
and LLM provider are active. This grows the same way the Go kernel's
config package did: as real providers (OpenAI, Gemini, a local model) get
added, EMBEDDING_PROVIDER / LLM_PROVIDER select between them at their
respective composition roots (app/api/embeddings.py, app/api/chat.py).
"""

import os
from dataclasses import dataclass


@dataclass(frozen=True)
class Settings:
    embedding_provider: str = os.getenv("EMBEDDING_PROVIDER", "mock")
    embedding_dimensions: int = int(os.getenv("EMBEDDING_DIMENSIONS", "8"))
    llm_provider: str = os.getenv("LLM_PROVIDER", "mock")


settings = Settings()