from fastapi import APIRouter, HTTPException

from app.config import settings
from app.llm.base import LLMProvider
from app.llm.mock import MockLLMProvider
from app.schemas.chat import ChatCompletionRequest, ChatCompletionResponse, ChatMessage

router = APIRouter()


def _build_provider() -> LLMProvider:
    """Selects the active LLMProvider based on settings.

    Only "mock" exists today. This is the seam where OpenAI/Gemini/local
    providers get added later — each new provider is a new branch here,
    nothing else in the API layer changes. Mirrors
    app/api/embeddings.py's _build_provider().
    """
    if settings.llm_provider == "mock":
        return MockLLMProvider()

    raise ValueError(f"unknown llm provider: {settings.llm_provider!r}")


_provider = _build_provider()


@router.post("/v1/chat/completions", response_model=ChatCompletionResponse)
async def create_chat_completion(request: ChatCompletionRequest) -> ChatCompletionResponse:
    if not request.messages:
        raise HTTPException(status_code=400, detail="messages must not be empty")

    answer = await _provider.complete(request.messages)
    return ChatCompletionResponse(message=ChatMessage(role="assistant", content=answer))