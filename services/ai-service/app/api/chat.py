from fastapi import APIRouter, HTTPException

from app.config import settings
from app.llm.base import LLMProvider
from app.llm.mock import MockLLMProvider
from app.llm.openrouter import OpenRouterProvider
from app.schemas.chat import ChatCompletionRequest, ChatCompletionResponse, ChatMessage

router = APIRouter()


def _build_provider() -> LLMProvider:
    """Selects the active LLMProvider based on settings.

    This is the seam where OpenAI/Gemini/local providers get added
    later — each new provider is a new branch here, nothing else in the
    API layer changes. Mirrors app/api/embeddings.py's _build_provider().
    """
    if settings.llm_provider == "mock":
        return MockLLMProvider()

    if settings.llm_provider == "openrouter":
        if not settings.openrouter_api_key:
            raise ValueError(
                "LLM_PROVIDER=openrouter requires OPENROUTER_API_KEY to be set"
            )
        return OpenRouterProvider(
            api_key=settings.openrouter_api_key,
            model=settings.openrouter_model,
        )

    raise ValueError(f"unknown llm provider: {settings.llm_provider!r}")


_provider = _build_provider()


@router.post("/v1/chat/completions", response_model=ChatCompletionResponse)
async def create_chat_completion(request: ChatCompletionRequest) -> ChatCompletionResponse:
    if not request.messages:
        raise HTTPException(status_code=400, detail="messages must not be empty")

    # Convert pydantic models to plain dicts at the API boundary, so
    # LLMProvider implementations never depend on FastAPI/pydantic types.
    messages = [{"role": m.role, "content": m.content} for m in request.messages]

    answer = await _provider.generate(messages)
    return ChatCompletionResponse(message=ChatMessage(role="assistant", content=answer))