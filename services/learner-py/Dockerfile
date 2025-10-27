# Builder stage - Install dependencies
FROM python:3.11-slim@sha256:1591aa8c01b5b37ab31dbe5662c5bdcf40c2f1bce4ef1c1fd24802dae3d01052 AS builder

ENV PYTHONUNBUFFERED=1 \
    POETRY_VERSION=1.7.1 \
    PIP_TIMEOUT=300 \
    PIP_RETRIES=5 \
    PIP_DEFAULT_TIMEOUT=300

# Install build dependencies
RUN apt-get update \
    && apt-get install -y --no-install-recommends build-essential curl \
    && rm -rf /var/lib/apt/lists/* \
    && pip install --no-cache-dir --timeout 300 --retries 5 "poetry==${POETRY_VERSION}"

WORKDIR /workspace/services/learner-py

# Copy dependency files (fail if poetry.lock is missing)
COPY pyproject.toml poetry.lock ./

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

# Copy source code
COPY . ./

# Install the package itself after copying source code
RUN poetry install --no-interaction --no-ansi --only-root

# Runtime stage - Minimal final image
FROM python:3.11-slim@sha256:1591aa8c01b5b37ab31dbe5662c5bdcf40c2f1bce4ef1c1fd24802dae3d01052

ENV PYTHONUNBUFFERED=1

# Install only runtime dependencies (no build tools)
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace/services/learner-py

# Copy installed Python packages from builder
COPY --from=builder /usr/local/lib/python3.11/site-packages /usr/local/lib/python3.11/site-packages
COPY --from=builder /workspace/services/learner-py ./

# Create non-root user and provision writable directories
RUN useradd -r -u 1000 -m -d /home/learner learner \
    && mkdir -p /checkpoints \
    && chown -R learner:learner /home/learner /workspace/services/learner-py /checkpoints

# Expose Prometheus metrics port
EXPOSE 9001

USER learner

# Use direct Python module execution instead of poetry run
ENTRYPOINT ["python", "-m", "learner"]
