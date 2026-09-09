from __future__ import annotations

import json
import os
import sqlite3
import uuid
from dataclasses import asdict, dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


class TaskNotFound(Exception):
    pass


class TaskConflict(Exception):
    pass


@dataclass
class TaskRecord:
    task_id: str
    user_id: str
    project_id: str
    asset_id: str
    job_id: str
    domain: str  # image, music, video, workflow
    action: str  # generate, separate, align, convert, edit, etc.
    status: str  # queued, running, completed, failed, canceled
    created_at: str
    updated_at: str
    duration_seconds: float = 0.0
    idempotency_key: str | None = None
    request_params: dict[str, Any] = field(default_factory=dict)
    artifact_ids: list[str] = field(default_factory=list)
    error: str | None = None

    @property
    def full_path(self) -> str:
        return f"{self.user_id}/{self.project_id}/{self.asset_id}/{self.job_id}"

    def to_dict(self) -> dict[str, Any]:
        d = asdict(self)
        d["full_path"] = self.full_path
        return d

    @classmethod
    def from_row(cls, row: sqlite3.Row) -> TaskRecord:
        params_str = row["request_params"] or "{}"
        try:
            params = json.loads(params_str)
        except Exception:
            params = {}

        artifacts_str = row["artifact_ids"] or "[]"
        try:
            artifacts = json.loads(artifacts_str)
        except Exception:
            artifacts = []

        keys = row.keys()
        asset_id = row["asset_id"] if "asset_id" in keys and row["asset_id"] else "default_asset"
        job_id = row["job_id"] if "job_id" in keys and row["job_id"] else row["task_id"]

        return cls(
            task_id=row["task_id"],
            user_id=row["user_id"],
            project_id=row["project_id"],
            asset_id=asset_id,
            job_id=job_id,
            domain=row["domain"],
            action=row["action"],
            status=row["status"],
            created_at=row["created_at"],
            updated_at=row["updated_at"],
            duration_seconds=float(row["duration_seconds"] or 0.0),
            idempotency_key=row["idempotency_key"],
            request_params=params,
            artifact_ids=artifacts,
            error=row["error"],
        )


@dataclass
class ProjectRecord:
    user_id: str
    project_id: str
    name: str
    description: str = ""
    created_at: str = ""
    updated_at: str = ""

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)

    @classmethod
    def from_row(cls, row: sqlite3.Row) -> ProjectRecord:
        return cls(
            user_id=row["user_id"],
            project_id=row["project_id"],
            name=row["name"],
            description=row["description"] or "",
            created_at=row["created_at"],
            updated_at=row["updated_at"],
        )


SCHEMA_SQL = """
CREATE TABLE IF NOT EXISTS projects (
    user_id TEXT NOT NULL,
    project_id TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY(user_id, project_id)
);

CREATE TABLE IF NOT EXISTS tasks (
    task_id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    project_id TEXT NOT NULL,
    asset_id TEXT NOT NULL DEFAULT 'default_asset',
    job_id TEXT NOT NULL DEFAULT '',
    domain TEXT NOT NULL,
    action TEXT NOT NULL,
    status TEXT NOT NULL,
    idempotency_key TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    duration_seconds REAL DEFAULT 0.0,
    request_params TEXT NOT NULL,
    artifact_ids TEXT DEFAULT '[]',
    error TEXT,
    UNIQUE(user_id, project_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_tasks_user_proj ON tasks(user_id, project_id);
CREATE INDEX IF NOT EXISTS idx_tasks_asset ON tasks(user_id, project_id, asset_id);
CREATE INDEX IF NOT EXISTS idx_tasks_job ON tasks(user_id, project_id, asset_id, job_id);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
CREATE INDEX IF NOT EXISTS idx_tasks_domain ON tasks(domain);
CREATE INDEX IF NOT EXISTS idx_tasks_created ON tasks(created_at DESC);
"""


class TaskStore:
    def __init__(self, db_path: Path) -> None:
        self.db_path = db_path

    @classmethod
    def from_environment(cls) -> TaskStore:
        root = Path(os.environ.get("STUDIO_ARTIFACT_ROOT", "/data/projects"))
        return cls(db_path=root / "studio_tasks.db")

    def ensure_ready(self) -> None:
        self.db_path.parent.mkdir(parents=True, exist_ok=True)
        with self._get_conn() as conn:
            conn.executescript(SCHEMA_SQL)
            conn.commit()

    def _get_conn(self) -> sqlite3.Connection:
        conn = sqlite3.connect(
            str(self.db_path),
            timeout=30.0,
            check_same_thread=False,
            isolation_level=None,
        )
        conn.row_factory = sqlite3.Row
        conn.execute("PRAGMA journal_mode=WAL;")
        conn.execute("PRAGMA busy_timeout=5000;")
        return conn

    # --- Project Operations ---

    def ensure_project(
        self, user_id: str, project_id: str, name: str | None = None, description: str = ""
    ) -> ProjectRecord:
        self.ensure_ready()
        now = datetime.now(timezone.utc).isoformat()
        pname = name or project_id
        with self._get_conn() as conn:
            conn.execute(
                """
                INSERT INTO projects (user_id, project_id, name, description, created_at, updated_at)
                VALUES (?, ?, ?, ?, ?, ?)
                ON CONFLICT(user_id, project_id) DO UPDATE SET updated_at = ?
                """,
                (user_id, project_id, pname, description, now, now, now),
            )
            row = conn.execute(
                "SELECT * FROM projects WHERE user_id = ? AND project_id = ?",
                (user_id, project_id),
            ).fetchone()
            return ProjectRecord.from_row(row)

    def list_projects(self, user_id: str | None = None) -> list[ProjectRecord]:
        self.ensure_ready()
        with self._get_conn() as conn:
            if user_id:
                rows = conn.execute(
                    "SELECT * FROM projects WHERE user_id = ? ORDER BY updated_at DESC",
                    (user_id,),
                ).fetchall()
            else:
                rows = conn.execute(
                    "SELECT * FROM projects ORDER BY updated_at DESC"
                ).fetchall()
            return [ProjectRecord.from_row(r) for r in rows]

    # --- Task Operations ---

    def create_task(
        self,
        *,
        user_id: str,
        project_id: str,
        asset_id: str | None = None,
        job_id: str | None = None,
        domain: str,
        action: str,
        idempotency_key: str | None = None,
        request_params: dict[str, Any] | None = None,
        task_id: str | None = None,
        status: str = "queued",
    ) -> TaskRecord:
        self.ensure_ready()
        self.ensure_project(user_id, project_id)

        now = datetime.now(timezone.utc).isoformat()
        resolved_asset_id = (asset_id or f"{domain}:{action}").strip()
        resolved_task_id = task_id or f"task_{domain[:3]}_{uuid.uuid4().hex[:16]}"
        resolved_job_id = (job_id or resolved_task_id).strip()
        params_json = json.dumps(request_params or {}, ensure_ascii=False)

        with self._get_conn() as conn:
            if idempotency_key:
                cursor = conn.execute(
                    "SELECT * FROM tasks WHERE user_id = ? AND project_id = ? AND idempotency_key = ?",
                    (user_id, project_id, idempotency_key),
                )
                row = cursor.fetchone()
                if row:
                    return TaskRecord.from_row(row)

            try:
                conn.execute(
                    """
                    INSERT INTO tasks (
                        task_id, user_id, project_id, asset_id, job_id, domain, action, status,
                        idempotency_key, created_at, updated_at, duration_seconds,
                        request_params, artifact_ids, error
                    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0.0, ?, '[]', NULL)
                    """,
                    (
                        resolved_task_id,
                        user_id,
                        project_id,
                        resolved_asset_id,
                        resolved_job_id,
                        domain,
                        action,
                        status,
                        idempotency_key,
                        now,
                        now,
                        params_json,
                    ),
                )
            except sqlite3.IntegrityError:
                if idempotency_key:
                    row = conn.execute(
                        "SELECT * FROM tasks WHERE user_id = ? AND project_id = ? AND idempotency_key = ?",
                        (user_id, project_id, idempotency_key),
                    ).fetchone()
                    if row:
                        return TaskRecord.from_row(row)
                raise TaskConflict(f"Task ID 冲突: {resolved_task_id}")

            row = conn.execute(
                "SELECT * FROM tasks WHERE task_id = ?", (resolved_task_id,)
            ).fetchone()
            return TaskRecord.from_row(row)

    def get_task(self, task_id: str) -> TaskRecord:
        self.ensure_ready()
        with self._get_conn() as conn:
            row = conn.execute(
                "SELECT * FROM tasks WHERE task_id = ?", (task_id,)
            ).fetchone()
            if not row:
                raise TaskNotFound(f"任务未找到: {task_id}")
            return TaskRecord.from_row(row)

    def update_task_status(
        self,
        task_id: str,
        status: str,
        duration_seconds: float | None = None,
        artifact_ids: list[str] | None = None,
        error: str | None = None,
    ) -> TaskRecord:
        self.ensure_ready()
        now = datetime.now(timezone.utc).isoformat()
        with self._get_conn() as conn:
            row = conn.execute("SELECT * FROM tasks WHERE task_id = ?", (task_id,)).fetchone()
            if not row:
                raise TaskNotFound(f"任务未找到: {task_id}")

            updates = ["status = ?", "updated_at = ?"]
            vals: list[Any] = [status, now]

            if duration_seconds is not None:
                updates.append("duration_seconds = ?")
                vals.append(duration_seconds)
            if artifact_ids is not None:
                updates.append("artifact_ids = ?")
                vals.append(json.dumps(artifact_ids, ensure_ascii=False))
            if error is not None:
                updates.append("error = ?")
                vals.append(error)

            vals.append(task_id)
            query = f"UPDATE tasks SET {', '.join(updates)} WHERE task_id = ?"
            conn.execute(query, tuple(vals))

            updated_row = conn.execute("SELECT * FROM tasks WHERE task_id = ?", (task_id,)).fetchone()
            return TaskRecord.from_row(updated_row)

    def list_tasks(
        self,
        *,
        user_id: str | None = None,
        project_id: str | None = None,
        asset_id: str | None = None,
        job_id: str | None = None,
        domain: str | None = None,
        status: str | None = None,
        limit: int = 50,
        offset: int = 0,
    ) -> tuple[list[TaskRecord], int]:
        self.ensure_ready()
        conds: list[str] = []
        params: list[Any] = []

        if user_id:
            conds.append("user_id = ?")
            params.append(user_id)
        if project_id:
            conds.append("project_id = ?")
            params.append(project_id)
        if asset_id:
            conds.append("asset_id = ?")
            params.append(asset_id)
        if job_id:
            conds.append("job_id = ?")
            params.append(job_id)
        if domain:
            conds.append("domain = ?")
            params.append(domain)
        if status:
            conds.append("status = ?")
            params.append(status)

        where = f"WHERE {' AND '.join(conds)}" if conds else ""

        with self._get_conn() as conn:
            count_row = conn.execute(f"SELECT COUNT(*) as cnt FROM tasks {where}", tuple(params)).fetchone()
            total = count_row["cnt"] if count_row else 0

            query = f"SELECT * FROM tasks {where} ORDER BY created_at DESC LIMIT ? OFFSET ?"
            rows = conn.execute(query, tuple(params + [limit, offset])).fetchall()
            return [TaskRecord.from_row(r) for r in rows], total

    def list_asset_jobs(
        self, user_id: str, project_id: str, asset_id: str, limit: int = 50
    ) -> list[TaskRecord]:
        """按 <User>/<Project>/<AssetID> 获取同一素材的所有生成 Job 历史列表."""
        tasks, _ = self.list_tasks(
            user_id=user_id, project_id=project_id, asset_id=asset_id, limit=limit
        )
        return tasks
