from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

from src.aggregator import MCPAggregator
from src.tasks import TaskStore


class TestMCPAggregator(unittest.TestCase):
    def setUp(self):
        self.temp_dir = tempfile.TemporaryDirectory()
        self.db_path = Path(self.temp_dir.name) / "test_tasks.db"
        self.store = TaskStore(self.db_path)
        self.aggregator = MCPAggregator(self.store)

    def tearDown(self):
        self.temp_dir.cleanup()

    def test_parse_user_project(self):
        # 显式提供 user_id
        uid, pid = self.aggregator.parse_user_project("my-proj", user_id="mengsk")
        self.assertEqual(uid, "mengsk")
        self.assertEqual(pid, "my-proj")

        # 复合 <User>/<Project> 格式
        uid, pid = self.aggregator.parse_user_project("mengsk/jyby-mv")
        self.assertEqual(uid, "mengsk")
        self.assertEqual(pid, "jyby-mv")

        # 默认回退
        uid, pid = self.aggregator.parse_user_project("simple-project")
        self.assertEqual(uid, "default")
        self.assertEqual(pid, "simple-project")


if __name__ == "__main__":
    unittest.main()
