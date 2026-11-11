FROM python:3.12-slim

ENV PYTHONUNBUFFERED=1 PIP_NO_CACHE_DIR=1 UV_NO_SYNC=1
WORKDIR /app
COPY pyproject.toml uv.lock ./
RUN pip install --no-cache-dir uv && uv sync --locked --no-dev --no-install-project
COPY openapi_worker ./openapi_worker
RUN uv sync --locked --no-dev
EXPOSE 8080
CMD ["uv", "run", "--no-sync", "python", "-m", "openapi_worker.app"]
