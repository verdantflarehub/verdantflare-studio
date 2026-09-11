from __future__ import annotations

import json
import os
import time
from typing import Any

import httpx
from mcp import types
from mcp.server.mcpserver import MCPServer

from .tasks import TaskRecord, TaskStore


class AggregatorError(Exception):
    pass


class MCPAggregator:
    def __init__(
        self,
        task_store: TaskStore,
        image_url: str | None = None,
        music_url: str | None = None,
        video_url: str | None = None,
    ) -> None:
        self.task_store = task_store
        self.image_url = (image_url or os.environ.get("IMAGE_MCP_URL", "http://image-mcp-server:8000")).rstrip("/")
        self.music_url = (music_url or os.environ.get("MUSIC_MCP_URL", "http://music-mcp-server:8000")).rstrip("/")
        self.video_url = (video_url or os.environ.get("VIDEO_MCP_URL", "http://video-mcp-server:8000")).rstrip("/")
        self.client = httpx.AsyncClient(timeout=120.0)

    @classmethod
    def from_environment(cls, task_store: TaskStore) -> MCPAggregator:
        return cls(
            task_store=task_store,
            image_url=os.environ.get("IMAGE_MCP_URL"),
            music_url=os.environ.get("MUSIC_MCP_URL"),
            video_url=os.environ.get("VIDEO_MCP_URL"),
        )

    def parse_user_project(self, project_id: str, user_id: str | None = None) -> tuple[str, str]:
        """解析 <User>/<Project> 复合格式或独立字段."""
        u, p, _, _ = self.parse_metadata_path(project_id, user_id=user_id)
        return u, p

    def parse_metadata_path(
        self,
        project_id: str,
        user_id: str | None = None,
        asset_id: str | None = None,
        job_id: str | None = None,
    ) -> tuple[str, str, str, str]:
        """解析 <User>/<Project>/<AssetID>/{JobID} 四段式元数据."""
        parts = [p.strip() for p in project_id.strip().split("/") if p.strip()]

        parsed_user = user_id.strip() if user_id else "default"
        parsed_project = project_id.strip()
        parsed_asset = asset_id.strip() if asset_id else "default_asset"
        parsed_job = job_id.strip() if job_id else ""

        if len(parts) >= 4:
            parsed_user, parsed_project, parsed_asset, parsed_job = parts[0], parts[1], parts[2], parts[3]
        elif len(parts) == 3:
            parsed_user, parsed_project, parsed_asset = parts[0], parts[1], parts[2]
        elif len(parts) == 2:
            parsed_user, parsed_project = parts[0], parts[1]

        if user_id:
            parsed_user = user_id.strip()
        if asset_id:
            parsed_asset = asset_id.strip()
        if job_id:
            parsed_job = job_id.strip()

        return parsed_user, parsed_project, parsed_asset, parsed_job

    async def _forward_mcp_tool(
        self,
        target_url: str,
        tool_name: str,
        arguments: dict[str, Any],
        user_id: str,
        project_id: str,
        task_id: str | None = None,
        asset_id: str | None = None,
        job_id: str | None = None,
    ) -> dict[str, Any]:
        """向内网各 App 发起 MCP JSON-RPC 工具调用，并注入受信任身份与资产版本头."""
        payload = {
            "jsonrpc": "2.0",
            "method": "tools/call",
            "params": {
                "name": tool_name,
                "arguments": arguments,
            },
            "id": int(time.time() * 1000),
        }
        headers = {
            "Content-Type": "application/json",
            "X-User-Id": user_id,
            "X-Project-Id": project_id,
        }
        if asset_id:
            headers["X-Asset-Id"] = asset_id
        if job_id:
            headers["X-Job-Id"] = job_id
        if task_id:
            headers["X-Task-Id"] = task_id

        # 尝试标准 /mcp 端点，备选 /
        for endpoint in ("/mcp", "/"):
            try:
                resp = await self.client.post(f"{target_url}{endpoint}", json=payload, headers=headers)
                if resp.status_code == 200:
                    data = resp.json()
                    if "error" in data:
                        raise AggregatorError(f"下游工具调用报错 [{tool_name}]: {data['error']}")
                    result = data.get("result", {})
                    # 优先提取 structuredContent，否则解析 content text
                    if "structuredContent" in result:
                        return result["structuredContent"]
                    content = result.get("content", [])
                    if content and isinstance(content, list) and "text" in content[0]:
                        try:
                            return json.loads(content[0]["text"])
                        except Exception:
                            return {"text": content[0]["text"]}
                    return result
            except httpx.RequestError as exc:
                if endpoint == "/":
                    raise AggregatorError(f"无法连接下游微服务 [{target_url}]: {exc}")
                continue
        raise AggregatorError(f"下游调用失败 [{target_url}]")


def register_aggregated_tools(mcp: MCPServer, aggregator: MCPAggregator, task_store: TaskStore) -> None:
    """在统一 MCPServer 上挂载 image, music, video 和 studio 聚合工具."""

    def _result(value: dict[str, Any]) -> types.CallToolResult:
        return types.CallToolResult(
            content=[types.TextContent(type="text", text=json.dumps(value, ensure_ascii=False))],
            structuredContent=value,
        )

    # ==================== 1. 图像工具集 (image.*) ====================

    @mcp.tool(name="image.generate")
    async def image_generate(
        project_id: str,
        idempotency_key: str,
        prompt: str,
        user_id: str | None = None,
        engine: str = "codex",
        model: str = "",
        aspect_ratio: str = "16:9",
        resolution: str = "2k",
        quality: str = "auto",
    ) -> types.CallToolResult:
        uid, pid = aggregator.parse_user_project(project_id, user_id)
        task = task_store.create_task(
            user_id=uid,
            project_id=pid,
            domain="image",
            action="generate",
            idempotency_key=idempotency_key,
            request_params={
                "prompt": prompt,
                "engine": engine,
                "model": model,
                "aspect_ratio": aspect_ratio,
                "resolution": resolution,
                "quality": quality,
            },
        )
        task_store.update_task_status(task.task_id, "running")

        try:
            downstream_args = {
                "project_id": pid,
                "idempotency_key": idempotency_key,
                "prompt": prompt,
                "engine": engine,
                "model": model,
                "aspect_ratio": aspect_ratio,
                "resolution": resolution,
                "quality": quality,
            }
            res = await aggregator._forward_mcp_tool(
                aggregator.image_url, "image.generate", downstream_args, uid, pid, task.task_id
            )
            return _result({"studio_task_id": task.task_id, "status": "running", "downstream": res})
        except Exception as e:
            task_store.update_task_status(task.task_id, "failed", error=str(e))
            return _result({"studio_task_id": task.task_id, "status": "failed", "error": str(e)})

    @mcp.tool(name="image.status")
    async def image_status(task_id: str) -> types.CallToolResult:
        try:
            task = task_store.get_task(task_id)
            return _result(task.to_dict())
        except Exception:
            # 兼容直接查询下游任务 ID
            res = await aggregator._forward_mcp_tool(
                aggregator.image_url, "image.status", {"task_id": task_id}, "default", "default"
            )
            return _result(res)

    @mcp.tool(name="image.result")
    async def image_result(task_id: str) -> types.CallToolResult:
        res = await aggregator._forward_mcp_tool(
            aggregator.image_url, "image.result", {"task_id": task_id}, "default", "default"
        )
        return _result(res)

    # ==================== 2. 音乐工具集 (music.*) ====================

    @mcp.tool(name="music.generate")
    async def music_generate(
        project_id: str,
        lyrics: str,
        instructions: str,
        candidate_number: int = 1,
        user_id: str | None = None,
        idempotency_key: str | None = None,
        seed: int = 7,
        max_duration_seconds: float = 200.0,
    ) -> types.CallToolResult:
        uid, pid = aggregator.parse_user_project(project_id, user_id)
        task = task_store.create_task(
            user_id=uid,
            project_id=pid,
            domain="music",
            action="generate",
            idempotency_key=idempotency_key,
            request_params={
                "lyrics": lyrics,
                "instructions": instructions,
                "candidate_number": candidate_number,
                "seed": seed,
                "max_duration_seconds": max_duration_seconds,
            },
        )
        task_store.update_task_status(task.task_id, "running")

        try:
            downstream_args = {
                "project_id": pid,
                "lyrics": lyrics,
                "instructions": instructions,
                "candidate_number": candidate_number,
                "seed": seed,
                "max_duration_seconds": max_duration_seconds,
            }
            res = await aggregator._forward_mcp_tool(
                aggregator.music_url, "music.generate", downstream_args, uid, pid, task.task_id
            )
            task_store.update_task_status(task.task_id, "completed")
            return _result({"studio_task_id": task.task_id, "status": "completed", "result": res})
        except Exception as e:
            task_store.update_task_status(task.task_id, "failed", error=str(e))
            return _result({"studio_task_id": task.task_id, "status": "failed", "error": str(e)})

    @mcp.tool(name="music.stems_separate")
    async def music_stems_separate(
        project_id: str, audio_asset_id: str, user_id: str | None = None
    ) -> types.CallToolResult:
        uid, pid = aggregator.parse_user_project(project_id, user_id)
        downstream_args = {"project_id": pid, "audio_asset_id": audio_asset_id}
        res = await aggregator._forward_mcp_tool(
            aggregator.music_url, "stems.separate", downstream_args, uid, pid
        )
        return _result(res)

    @mcp.tool(name="music.voice_convert")
    async def music_voice_convert(
        project_id: str,
        audio_asset_id: str,
        model_id: str,
        user_id: str | None = None,
        pitch_shift: int = 0,
    ) -> types.CallToolResult:
        uid, pid = aggregator.parse_user_project(project_id, user_id)
        downstream_args = {
            "project_id": pid,
            "audio_asset_id": audio_asset_id,
            "model_id": model_id,
            "pitch_shift": pitch_shift,
        }
        res = await aggregator._forward_mcp_tool(
            aggregator.music_url, "voice.convert", downstream_args, uid, pid
        )
        return _result(res)

    @mcp.tool(name="music.lyrics_align")
    async def music_lyrics_align(
        project_id: str,
        vocal_asset_id: str,
        lyrics: str,
        language: str = "zh",
        user_id: str | None = None,
    ) -> types.CallToolResult:
        uid, pid = aggregator.parse_user_project(project_id, user_id)
        downstream_args = {
            "project_id": pid,
            "vocal_asset_id": vocal_asset_id,
            "lyrics": lyrics,
            "language": language,
        }
        res = await aggregator._forward_mcp_tool(
            aggregator.music_url, "lyrics.align", downstream_args, uid, pid
        )
        return _result(res)

    @mcp.tool(name="music.mix_master")
    async def music_mix_master(
        project_id: str,
        instrumental_asset_id: str,
        vocal_asset_id: str,
        lyrics_lrc: str,
        bpm: float,
        user_id: str | None = None,
    ) -> types.CallToolResult:
        uid, pid = aggregator.parse_user_project(project_id, user_id)
        downstream_args = {
            "project_id": pid,
            "instrumental_asset_id": instrumental_asset_id,
            "vocal_asset_id": vocal_asset_id,
            "lyrics_lrc": lyrics_lrc,
            "bpm": bpm,
        }
        res = await aggregator._forward_mcp_tool(
            aggregator.music_url, "mix.master", downstream_args, uid, pid
        )
        return _result(res)

    # ==================== 3. 视频工具集 (video.*) ====================

    @mcp.tool(name="video.generate")
    async def video_generate(
        project_id: str,
        idempotency_key: str,
        model: str,
        prompt: str,
        duration_seconds: int,
        aspect_ratio: str,
        references: dict[str, list[dict[str, str]]],
        user_id: str | None = None,
    ) -> types.CallToolResult:
        uid, pid = aggregator.parse_user_project(project_id, user_id)
        task = task_store.create_task(
            user_id=uid,
            project_id=pid,
            domain="video",
            action="generate",
            idempotency_key=idempotency_key,
            request_params={
                "model": model,
                "prompt": prompt,
                "duration_seconds": duration_seconds,
                "aspect_ratio": aspect_ratio,
            },
        )
        task_store.update_task_status(task.task_id, "running")

        try:
            downstream_args = {
                "project_id": pid,
                "idempotency_key": idempotency_key,
                "model": model,
                "prompt": prompt,
                "duration_seconds": duration_seconds,
                "aspect_ratio": aspect_ratio,
                "references": references,
            }
            res = await aggregator._forward_mcp_tool(
                aggregator.video_url, "video.generate", downstream_args, uid, pid, task.task_id
            )
            return _result({"studio_task_id": task.task_id, "status": "running", "downstream": res})
        except Exception as e:
            task_store.update_task_status(task.task_id, "failed", error=str(e))
            return _result({"studio_task_id": task.task_id, "status": "failed", "error": str(e)})

    @mcp.tool(name="video.status")
    async def video_status(video_task_id: str) -> types.CallToolResult:
        res = await aggregator._forward_mcp_tool(
            aggregator.video_url, "video.status", {"video_task_id": video_task_id}, "default", "default"
        )
        return _result(res)

    @mcp.tool(name="video.result")
    async def video_result(video_task_id: str) -> types.CallToolResult:
        res = await aggregator._forward_mcp_tool(
            aggregator.video_url, "video.result", {"video_task_id": video_task_id}, "default", "default"
        )
        return _result(res)

    # ==================== 4. 跨模态项目与资产工具 ====================

    @mcp.tool(name="project.create")
    def project_create(name: str, user_id: str = "default", description: str = "") -> types.CallToolResult:
        proj = task_store.ensure_project(user_id=user_id, project_id=name, name=name, description=description)
        return _result({"status": "created", "project": proj.to_dict()})

    @mcp.tool(name="task.list")
    def task_list(
        project_id: str | None = None,
        user_id: str | None = None,
        domain: str | None = None,
        status: str | None = None,
        limit: int = 50,
        offset: int = 0,
    ) -> types.CallToolResult:
        records, total = task_store.list_tasks(
            user_id=user_id,
            project_id=project_id,
            domain=domain,
            status=status,
            limit=limit,
            offset=offset,
        )
        return _result(
            {
                "tasks": [r.to_dict() for r in records],
                "total": total,
                "limit": limit,
                "offset": offset,
            }
        )
