import os
import re
from sqlalchemy import create_engine
from sqlalchemy.orm import sessionmaker

_DSN_RAW = os.environ.get(
    "DATABASE_DSN",
    "root:password@tcp(127.0.0.1:3306)/opinion_analysis",
)


def _to_sqlalchemy_dsn(raw: str) -> str:
    if raw.startswith("mysql"):
        return raw
    # GORM format: user:pass@tcp(host:port)/db?params
    m = re.match(r"(.+):(.+)@tcp\((.+):(\d+)\)/(\w+)", raw)
    if m:
        user, pwd, host, port, db = m.groups()
        return f"mysql+pymysql://{user}:{pwd}@{host}:{port}/{db}?charset=utf8mb4"
    raise ValueError(f"Unrecognised DATABASE_DSN format: {raw!r}")


engine = create_engine(_to_sqlalchemy_dsn(_DSN_RAW), pool_pre_ping=True)
SessionLocal = sessionmaker(bind=engine)
