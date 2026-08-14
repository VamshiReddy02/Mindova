from fastapi.testclient import TestClient

from app.main import app

client = TestClient(app)


def test_empty_texts():
    response = client.post("/v1/embeddings", json={"texts": []})
    assert response.status_code == 400


def test_single_text():
    response = client.post(
        "/v1/embeddings",
        json={"texts": ["Mindova is an AI knowledge platform."]},
    )
    assert response.status_code == 200
    data = response.json()
    assert len(data["embeddings"]) == 1


def test_multiple_texts():
    texts = [
        "Mindova is an AI knowledge platform.",
        "Documents are stored in PostgreSQL.",
    ]
    response = client.post("/v1/embeddings", json={"texts": texts})
    assert response.status_code == 200
    data = response.json()
    assert len(data["embeddings"]) == 2


def test_embedding_count_matches_input():
    texts = ["a", "b", "c", "d", "e"]
    response = client.post("/v1/embeddings", json={"texts": texts})
    assert response.status_code == 200
    data = response.json()
    assert len(data["embeddings"]) == len(texts)


def test_embedding_dimension_is_8():
    response = client.post("/v1/embeddings", json={"texts": ["hello"]})
    assert response.status_code == 200
    data = response.json()
    assert len(data["embeddings"][0]) == 8


def test_same_text_is_deterministic():
    text = "same text every time"

    first = client.post("/v1/embeddings", json={"texts": [text]})
    second = client.post("/v1/embeddings", json={"texts": [text]})

    assert first.status_code == 200
    assert second.status_code == 200
    assert first.json()["embeddings"][0] == second.json()["embeddings"][0]


def test_different_texts_produce_different_vectors():
    response = client.post(
        "/v1/embeddings",
        json={"texts": ["completely different text one", "completely different text two"]},
    )
    assert response.status_code == 200

    embeddings = response.json()["embeddings"]
    assert embeddings[0] != embeddings[1]