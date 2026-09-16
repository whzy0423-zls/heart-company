# Nine-Xing Knowledge Service

Internal FastAPI/LangChain service for scoped retrieval and answer generation.
The Go gateway remains responsible for authentication, authorization, billing,
conversation persistence, and selecting immutable knowledge release IDs.

## Local development

```bash
python3 -m venv .venv
.venv/bin/pip install -e '.[test]'
LANGCHAIN_SERVICE_TOKEN=TOKEN .venv/bin/uvicorn app.main:app --port 8081
.venv/bin/pytest -q
```

The service is internal-only. Do not expose port 8081 publicly or pass App
login tokens to it.

