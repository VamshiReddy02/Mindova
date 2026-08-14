from fastapi import FastAPI

from app.api.embeddings import router as embeddings_router

app = FastAPI(title="Mindova AI Service", version="0.1.0")

app.include_router(embeddings_router)


@app.get("/health")
async def health() -> dict[str, str]:
    return {"status": "ok"}