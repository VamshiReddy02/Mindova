from fastapi.testclient import TestClient

from app.main import app

client = TestClient(app)


def test_empty_messages():
    response = client.post("/v1/chat/completions", json={"messages": []})
    assert response.status_code == 400


def test_single_user_message():
    response = client.post(
        "/v1/chat/completions",
        json={"messages": [{"role": "user", "content": "How does Mindova process documents?"}]},
    )
    assert response.status_code == 200

    data = response.json()
    assert data["message"]["role"] == "assistant"
    assert len(data["message"]["content"]) > 0


def test_response_includes_the_question():
    question = "What is pgvector used for?"
    response = client.post(
        "/v1/chat/completions",
        json={"messages": [{"role": "user", "content": question}]},
    )
    assert response.status_code == 200

    data = response.json()
    assert question in data["message"]["content"]


def test_system_context_is_reflected_in_the_response():
    response = client.post(
        "/v1/chat/completions",
        json={
            "messages": [
                {"role": "system", "content": "Context: Mindova stores chunks in PostgreSQL."},
                {"role": "user", "content": "Where are chunks stored?"},
            ]
        },
    )
    assert response.status_code == 200

    data = response.json()
    assert "context" in data["message"]["content"].lower()


def test_multi_turn_conversation_uses_the_latest_user_message():
    response = client.post(
        "/v1/chat/completions",
        json={
            "messages": [
                {"role": "user", "content": "first question, ignored"},
                {"role": "assistant", "content": "an earlier answer"},
                {"role": "user", "content": "second question, the real one"},
            ]
        },
    )
    assert response.status_code == 200

    data = response.json()
    assert "second question, the real one" in data["message"]["content"]
    assert "first question, ignored" not in data["message"]["content"]


def test_deterministic_for_same_input():
    body = {"messages": [{"role": "user", "content": "same question every time"}]}

    first = client.post("/v1/chat/completions", json=body)
    second = client.post("/v1/chat/completions", json=body)

    assert first.status_code == 200
    assert second.status_code == 200
    assert first.json()["message"]["content"] == second.json()["message"]["content"]


def test_only_assistant_messages_and_content_are_returned():
    response = client.post(
        "/v1/chat/completions",
        json={"messages": [{"role": "user", "content": "hello"}]},
    )
    assert response.status_code == 200

    data = response.json()
    assert set(data["message"].keys()) == {"role", "content"}