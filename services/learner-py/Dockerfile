FROM python:3.11-slim AS base

ENV PYTHONUNBUFFERED=1 \
    POETRY_VERSION=1.7.1

RUN apt-get update \
    && apt-get install -y --no-install-recommends build-essential \
    && rm -rf /var/lib/apt/lists/* \
    && pip install --no-cache-dir "poetry==${POETRY_VERSION}"

WORKDIR /workspace/services/learner-py

COPY services/learner-py/pyproject.toml services/learner-py/poetry.lock ./

RUN poetry config virtualenvs.create false \
    && poetry install --no-interaction --no-ansi --only main

COPY services/learner-py/ ./

ENTRYPOINT ["poetry", "run", "learner"]
