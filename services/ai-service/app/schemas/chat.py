from pydantic import BaseModel, Field


class ChatMessage(BaseModel):
    role: str  # "system" | "user" | "assistant"
    content: str


class ChatCompletionRequest(BaseModel):
    messages: list[ChatMessage] = Field(default_factory=list)


class ChatCompletionResponse(BaseModel):
    message: ChatMessage
