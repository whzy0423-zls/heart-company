from functools import lru_cache

from pydantic import Field, SecretStr
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    service_token: SecretStr = SecretStr("")
    database_url: str = Field(default="", validation_alias="DATABASE_URL")
    embedding_api_base: str = Field(default="", validation_alias="EMBEDDING_API_BASE")
    embedding_api_key: SecretStr = Field(default=SecretStr(""), validation_alias="EMBEDDING_API_KEY")
    embedding_model: str = Field(default="", validation_alias="EMBEDDING_MODEL")
    embedding_dimension: int = Field(default=1024, validation_alias="EMBEDDING_DIMENSION", gt=0)

    model_config = SettingsConfigDict(
        env_prefix="LANGCHAIN_",
        env_file=".env",
        extra="ignore",
    )


@lru_cache
def get_settings() -> Settings:
    return Settings()
