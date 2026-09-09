from __future__ import annotations

import contextlib
import hmac
import os
from typing import Any

from mcp.server.mcpserver import MCPServer
from mcp.server.transport_security import TransportSecuritySettings
from starlette.applications import Starlette
from starlette.middleware import Middleware
from starlette.middleware.base import BaseHTTPMiddleware
from starlette.requests import Request
from starlette.responses import JSONResponse, Response
from starlette.routing import Mount, Route

from .aggregator import MCPAggregator, register_aggregated_tools
from .dashboard import api_list_projects, api_list_tasks, dashboard_page
from .tasks import TaskStore

task_store = TaskStore.from_environment()
aggregator = MCPAggregator.from_environment(task_store)
mcp = MCPServer("VerdantFlare Studio")
register_aggregated_tools(mcp, aggregator, task_store)


def transport_security_from_environment() -> TransportSecuritySettings:
    hosts = [x.strip() for x in os.environ.get("STUDIO_MCP_ALLOWED_HOSTS", "").split(",") if x.strip()]
    origins = [x.strip() for x in os.environ.get("STUDIO_MCP_ALLOWED_ORIGINS", "").split(",") if x.strip()]
    return TransportSecuritySettings(
        enable_dns_rebinding_protection=True,
        allowed_hosts=hosts or ["127.0.0.1:*", "localhost:*", "[::1]:*"],
        allowed_origins=origins or ["http://127.0.0.1:*", "http://localhost:*", "http://[::1]:*"],
    )


async def health(request: Request) -> JSONResponse:
    task_store.ensure_ready()
    return JSONResponse(
        {
            "status": "ok",
            "service": "verdantflare-studio",
            "version": "v0.1.0",
            "downstream": {
                "image": aggregator.image_url,
                "music": aggregator.music_url,
                "video": aggregator.video_url,
            },
        }
    )


class BearerAuthMiddleware(BaseHTTPMiddleware):
    async def dispatch(self, request: Request, call_next):
        path = request.url.path
        # 健康检查与看板直接放行
        if path in ("/health", "/dashboard", "/dashboard/") or path.startswith("/api/"):
            return await call_next(request)

        token = os.environ.get("STUDIO_BEARER_TOKEN", "").strip()
        if token:
            auth_header = request.headers.get("authorization", "")
            query_token = request.query_params.get("token", "")
            valid_header = auth_header and hmac.compare_digest(auth_header, f"Bearer {token}")
            valid_query = query_token and hmac.compare_digest(query_token, token)
            if not (valid_header or valid_query):
                return JSONResponse({"error": "unauthorized"}, status_code=401)

        return await call_next(request)


@contextlib.asynccontextmanager
async def lifespan(app: Starlette):
    task_store.ensure_ready()
    app.state.task_store = task_store
    app.state.aggregator = aggregator
    try:
        async with mcp.session_manager.run():
            yield
    except RuntimeError as e:
        if "can only be called once" in str(e):
            yield
        else:
            raise


mcp_app = mcp.streamable_http_app(transport_security=transport_security_from_environment())

app = Starlette(
    routes=[
        Route("/health", health, methods=["GET"]),
        Route("/dashboard", dashboard_page, methods=["GET"]),
        Route("/dashboard/", dashboard_page, methods=["GET"]),
        Route("/api/tasks", api_list_tasks, methods=["GET"]),
        Route("/api/projects", api_list_projects, methods=["GET"]),
        Mount("/mcp", app=mcp_app),
        Mount("/", app=mcp_app),
    ],
    middleware=[
        Middleware(BearerAuthMiddleware),
    ],
    lifespan=lifespan,
)
