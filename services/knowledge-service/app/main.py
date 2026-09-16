from fastapi import FastAPI

from app.api.health import router as health_router
from app.api.retrieval import router as retrieval_router


app = FastAPI(title="Nine-Xing Knowledge Service", version="0.1.0")
app.include_router(health_router)
app.include_router(retrieval_router)

