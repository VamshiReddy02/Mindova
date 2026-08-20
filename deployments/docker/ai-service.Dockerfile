# syntax=docker/dockerfile:1

# ---- Builder -----------------------------------------------------------
#
# Installs dependencies into an isolated prefix (/install), kept separate
# from the runtime stage so build-only tooling (pip's own dependency
# resolution machinery, wheel caches, etc.) never ends up in the final
# image.
FROM python:3.12-slim AS builder

WORKDIR /build

COPY services/ai-service/requirements.txt .

# --prefix, not a venv: copying a plain prefix directory into the
# runtime stage's /usr/local is simpler than activating a venv across
# stages, and produces the same result — installed packages importable
# by the runtime stage's own Python interpreter.
RUN pip install --no-cache-dir --prefix=/install -r requirements.txt

# ---- Runtime -------------------------------------------------------------
#
# Same python:3.12-slim base as the builder (not distroless): the
# official Python distroless images are less consistently maintained
# than Go's, and pip-installed packages need to match the runtime
# interpreter's ABI closely enough that fighting that mismatch isn't
# worth it for an MVP. slim is still meaningfully smaller than the full
# python:3.12 image (no build toolchain, no extra CLI utilities) while
# staying on well-trodden, well-documented ground.
FROM python:3.12-slim AS runtime

# Copy the installed dependencies from the builder stage.
COPY --from=builder /install /usr/local

# Non-root user with an EXPLICIT numeric UID/GID (1000), not just a
# name. This matters specifically for Kubernetes: when a Pod sets
# securityContext.runAsNonRoot: true, the kubelet needs to verify the
# container's default user is non-root WITHOUT running it first — that
# requires a numeric UID directly in the image's metadata. A named user
# (USER app) can't be resolved that way (Kubernetes would have to read
# /etc/passwd inside a running container to find out, which defeats the
# whole point of a pre-execution safety check), and Kubernetes correctly
# refuses to start the Pod rather than guess:
#   "container has runAsNonRoot and image has non-numeric user (app),
#    cannot verify user is non-root"
# USER 1000 (numeric) sidesteps this entirely — same non-root user in
# practice, just identified by a number Kubernetes can check directly.
RUN addgroup --system --gid 1000 app \
 && adduser --system --uid 1000 --ingroup app app

WORKDIR /app

# Only the application source — never requirements.txt's build
# artifacts, never a .env file (excluded by the repo's .dockerignore
# regardless), never anything containing a real API key. The container
# has zero OpenRouter credentials baked in by design: OPENROUTER_API_KEY,
# LLM_PROVIDER, and OPENROUTER_MODEL are only ever read from the process
# environment at runtime (see app/config.py), supplied later via
# `docker run -e ...` or a Kubernetes Secret — never via an ENV
# instruction in this Dockerfile, and never committed to the image layer
# history.
COPY services/ai-service/app ./app

USER 1000

EXPOSE 8001

# LLM_PROVIDER defaults to "mock" (see app/config.py) if the environment
# variable isn't set, so this container is safe and functional to run
# with zero configuration — it just won't call a real LLM until
# OPENROUTER_API_KEY and LLM_PROVIDER=openrouter are supplied at runtime.
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8001"]
