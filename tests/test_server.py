from __future__ import annotations

import os
import tempfile
import unittest
from pathlib import Path

from starlette.testclient import TestClient

temp_dir = tempfile.TemporaryDirectory()
os.environ["STUDIO_ARTIFACT_ROOT"] = temp_dir.name
os.environ["STUDIO_BEARER_TOKEN"] = "test-secret-token"

from src.server import app


class TestStudioServer(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls._client_cm = TestClient(app)
        cls.client = cls._client_cm.__enter__()

    @classmethod
    def tearDownClass(cls):
        try:
            cls._client_cm.__exit__(None, None, None)
        finally:
            temp_dir.cleanup()

    def test_health_endpoint(self):
        resp = self.client.get("/health")
        self.assertEqual(resp.status_code, 200)
        data = resp.json()
        self.assertEqual(data["status"], "ok")
        self.assertEqual(data["service"], "verdantflare-studio")
        self.assertIn("downstream", data)

    def test_dashboard_endpoint(self):
        resp = self.client.get("/dashboard")
        self.assertEqual(resp.status_code, 200)
        self.assertIn("VerdantFlare Studio", resp.text)
        self.assertIn("Image 看板", resp.text)
        self.assertIn("Music 探针", resp.text)
        self.assertIn("Video H3", resp.text)

    def test_api_projects_and_tasks(self):
        resp = self.client.get("/api/projects")
        self.assertEqual(resp.status_code, 200)
        self.assertIn("projects", resp.json())

        resp = self.client.get("/api/tasks")
        self.assertEqual(resp.status_code, 200)
        self.assertIn("tasks", resp.json())

    def test_bearer_auth(self):
        # 保护路径未传 token -> 401
        resp = self.client.post("/mcp", json={})
        self.assertEqual(resp.status_code, 401)

        # 传错误 token -> 401
        resp = self.client.post("/mcp", json={}, headers={"Authorization": "Bearer wrong-token"})
        self.assertEqual(resp.status_code, 401)

        # 传正确 token -> 放行 (由于空 json 不符合 MCP JSON-RPC，可能报 400 但不报 401)
        headers = {"Authorization": "Bearer test-secret-token"}
        resp = self.client.post("/mcp", json={"jsonrpc": "2.0", "method": "ping", "id": 1}, headers=headers)
        self.assertNotEqual(resp.status_code, 401)


if __name__ == "__main__":
    unittest.main()
