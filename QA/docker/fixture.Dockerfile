# syntax=docker/dockerfile:1

ARG PYTHON_IMAGE_TAG=3.10.16-slim
FROM python:${PYTHON_IMAGE_TAG} AS qa-base

ARG POETRY_VERSION=2.3.2
ARG CLAUDE_CODE_VERSION=2.1.76

ENV DEBIAN_FRONTEND=noninteractive \
    HOME=/home/qa \
    PIP_DISABLE_PIP_VERSION_CHECK=1 \
    POETRY_NO_INTERACTION=1 \
    POETRY_VIRTUALENVS_CREATE=true \
    POETRY_VIRTUALENVS_IN_PROJECT=true \
    PATH=/home/qa/.local/bin:${PATH}

RUN apt-get update \
 && apt-get install -y --no-install-recommends \
    bash \
    build-essential \
    ca-certificates \
    curl \
    git \
    libgl1 \
    libglib2.0-0 \
    libsm6 \
    libxext6 \
    libxrender1 \
    nodejs \
    npm \
 && rm -rf /var/lib/apt/lists/*

RUN pip install --no-cache-dir "poetry==${POETRY_VERSION}" \
 && npm install -g "@anthropic-ai/claude-code@${CLAUDE_CODE_VERSION}" \
 && npm cache clean --force

RUN git config --system user.name "Concierge QA" \
 && git config --system user.email "qa@example.com" \
 && useradd --create-home --shell /bin/bash qa \
 && mkdir -p /workspace \
 && chown -R qa:qa /workspace /home/qa

WORKDIR /workspace
USER qa

FROM qa-base AS fixture-cold

ARG FIXTURE_ID=unknown
ARG FIXTURE_REF=unknown

LABEL io.tensorleap.concierge.qa.fixture-id="${FIXTURE_ID}" \
      io.tensorleap.concierge.qa.fixture-ref="${FIXTURE_REF}" \
      io.tensorleap.concierge.qa.mode="cold"

COPY --chown=qa:qa bin/concierge /usr/local/bin/concierge
COPY --chown=qa:qa workspace/ /workspace/

CMD ["sleep", "infinity"]

FROM fixture-cold AS fixture-prewarmed

LABEL io.tensorleap.concierge.qa.mode="prewarmed"

RUN if [ -f pyproject.toml ]; then poetry env use python && poetry install --no-root; fi \
 && if [ -f .checkpoint_warmup.sh ]; then chmod +x .checkpoint_warmup.sh && ./.checkpoint_warmup.sh; fi

CMD ["sleep", "infinity"]
