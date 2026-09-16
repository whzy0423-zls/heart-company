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

## Index and evaluate

Set `DATABASE_URL`, `EMBEDDING_API_BASE`, `EMBEDDING_API_KEY`,
`EMBEDDING_MODEL=BAAI/bge-m3`, and `EMBEDDING_DIMENSION=1024`, then run:

```bash
python scripts/catalog_books.py SOURCE_DIR var/knowledge/catalog.jsonl
python scripts/ingest_books.py var/knowledge/catalog.jsonl var/knowledge/chunks.jsonl --limit 20
python scripts/index_chunks.py var/knowledge/chunks.jsonl --index-version bge-m3-v1
python scripts/build_evaluation.py var/knowledge/evaluation-500.jsonl
python scripts/evaluate.py var/knowledge/evaluation-500.jsonl --report var/knowledge/evaluation-report.json
```
