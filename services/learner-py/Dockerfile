FROM python:3.11-slim AS base

ENV PYTHONUNBUFFERED=1 \
    POETRY_VERSION=1.7.1 \
    PIP_TIMEOUT=300 \
    PIP_RETRIES=5 \
    PIP_DEFAULT_TIMEOUT=300

RUN apt-get update \
    && apt-get install -y --no-install-recommends build-essential curl \
    && rm -rf /var/lib/apt/lists/* \
    && pip install --no-cache-dir --timeout 300 --retries 5 "poetry==${POETRY_VERSION}"

WORKDIR /workspace/services/learner-py

COPY pyproject.toml poetry.lock* ./

# Configure poetry with better timeout and retry settings
RUN poetry config virtualenvs.create false \
    && poetry config installer.max-workers 1

# Configure pip for poetry to use with better timeouts
RUN pip config set global.timeout 600 \
    && pip config set global.retries 5

# Install CPU-only torch first to avoid CUDA package issues during development
RUN pip install --timeout 300 --retries 5 torch --index-url https://download.pytorch.org/whl/cpu

# Install dependencies only first
RUN poetry install --no-interaction --no-ansi --only main --no-root

COPY . ./

# Install the package itself after copying source code
RUN poetry install --no-interaction --no-ansi --only-root

ENTRYPOINT ["poetry", "run", "learner"]
