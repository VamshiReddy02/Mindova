from fastapi import APIRouter, HTTPException

from app.config import settings
from app.embedding.base import EmbeddingProvider
from app.embedding.mock import MockEmbeddingProvider
from app.schemas.embedding import EmbeddingRequest, EmbeddingResponse

router = APIRouter()


def _build_provider() -> EmbeddingProvider:
    """Selects the active EmbeddingProvider based on settings.

    Only "mock" exists today. This is the seam where OpenAI/Gemini/local
    providers get added later — each new provider is a new branch here,
    nothing else in the API layer changes.
    """
    if settings.embedding_provider == "mock":
        return MockEmbeddingProvider(dimensions=settings.embedding_dimensions)

    raise ValueError(f"unknown embedding provider: {settings.embedding_provider!r}")


# A single shared provider instance for the process lifetime. Fine for a
# stateless mock; a real provider with its own connection pooling/auth
# would likely still be constructed once here and reused the same way.
_provider = _build_provider()


@router.post("/v1/embeddings", response_model=EmbeddingResponse)
async def create_embeddings(request: EmbeddingRequest) -> EmbeddingResponse:
    if not request.texts:
        raise HTTPException(status_code=400, detail="texts must not be empty")

    embeddings = await _provider.embed(request.texts)
    return EmbeddingResponse(embeddings=embeddings)