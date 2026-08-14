"""Application settings, read from environment variables.

Deliberately minimal for now — just enough to configure which embedding
and LLM provider are active. This grows the same way the Go kernel's
config package did: as real providers (OpenAI, Gemini, a local model) get
added, EMBEDDING_PROVIDER / LLM_PROVIDER select between them at their
respective composition roots (app/api/embeddings.py, app/api/chat.py).

LLM_PROVIDER defaults to "mock" deliberately: local development and CI
never accidentally burns OpenRouter's free-tier quota (50 requests/day)
just because an env var wasn't set.
"""

import os
from dataclasses import dataclass


@dataclass(frozen=True)
class Settings:
    embedding_provider: str = os.getenv("EMBEDDING_PROVIDER", "mock")
    embedding_dimensions: int = int(os.getenv("EMBEDDING_DIMENSIONS", "8"))

    llm_provider: str = os.getenv("LLM_PROVIDER", "mock")
    openrouter_api_key: str = os.getenv("OPENROUTER_API_KEY", "")
    openrouter_model: str = os.getenv("OPENROUTER_MODEL", "openrouter/free")


settings = Settings()