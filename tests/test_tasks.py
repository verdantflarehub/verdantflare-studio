from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

from src.tasks import TaskConflict, TaskNotFound, TaskStore


class TestTaskStore(unittest.TestCase):
    def setUp(self):
        self.temp_dir = tempfile.TemporaryDirectory()
        self.db_path = Path(self.temp_dir.name) / "test_tasks.db"
        self.store = TaskStore(self.db_path)
        self.store.ensure_ready()

    def tearDown(self):
        self.temp_dir.cleanup()

    def test_ensure_project_and_list(self):
        p1 = self.store.ensure_project("mengsk", "jyby-mv", name="加油吧小月")
        self.assertEqual(p1.user_id, "mengsk")
        self.assertEqual(p1.project_id, "jyby-mv")
        self.assertEqual(p1.name, "加油吧小月")

        projs = self.store.list_projects("mengsk")
        self.assertEqual(len(projs), 1)
        self.assertEqual(projs[0].project_id, "jyby-mv")

    def test_create_and_get_task(self):
        task = self.store.create_task(
            user_id="mengsk",
            project_id="jyby-mv",
            domain="image",
            action="generate",
            idempotency_key="B01/wardrobe-v1",
            request_params={"prompt": "test prompt"},
        )
        self.assertTrue(task.task_id.startswith("task_ima_"))
        self.assertEqual(task.status, "queued")

        # Idempotent retrieval
        task2 = self.store.create_task(
            user_id="mengsk",
            project_id="jyby-mv",
            domain="image",
            action="generate",
            idempotency_key="B01/wardrobe-v1",
        )
        self.assertEqual(task.task_id, task2.task_id)

        # Direct get
        fetched = self.store.get_task(task.task_id)
        self.assertEqual(fetched.task_id, task.task_id)
        self.assertEqual(fetched.request_params["prompt"], "test prompt")

    def test_update_task_status(self):
        task = self.store.create_task(
            user_id="mengsk",
            project_id="jyby-mv",
            domain="video",
            action="generate",
        )
        updated = self.store.update_task_status(
            task.task_id,
            status="completed",
            duration_seconds=12.5,
            artifact_ids=["art_vid_123"],
        )
        self.assertEqual(updated.status, "completed")
        self.assertEqual(updated.duration_seconds, 12.5)
        self.assertEqual(updated.artifact_ids, ["art_vid_123"])

    def test_list_tasks_filtering(self):
        self.store.create_task(user_id="u1", project_id="p1", domain="music", action="generate")
        self.store.create_task(user_id="u1", project_id="p1", domain="image", action="generate")
        self.store.create_task(user_id="u2", project_id="p2", domain="video", action="generate")

        tasks, total = self.store.list_tasks(user_id="u1")
        self.assertEqual(total, 2)
        self.assertEqual(len(tasks), 2)

        music_tasks, total_m = self.store.list_tasks(domain="music")
        self.assertEqual(total_m, 1)
        self.assertEqual(music_tasks[0].domain, "music")

    def test_asset_multiple_job_iterations(self):
        # 验证一个素材 (<AssetID>) 经历多次生成 ({JobID}) 的场景
        user = "mengsk"
        proj = "jyby-mv"
        asset = "video:shot-b01"

        # 第一次生成: 4NFE 草稿 (Job 1)
        j1 = self.store.create_task(
            user_id=user,
            project_id=proj,
            asset_id=asset,
            job_id="job_draft_01",
            domain="video",
            action="generate",
            request_params={"steps": 4},
        )
        self.assertEqual(j1.full_path, "mengsk/jyby-mv/video:shot-b01/job_draft_01")
        self.store.update_task_status(j1.task_id, "completed", duration_seconds=18.0)

        # 第二次生成: 8NFE 高清成片 (Job 2)
        j2 = self.store.create_task(
            user_id=user,
            project_id=proj,
            asset_id=asset,
            job_id="job_final_02",
            domain="video",
            action="generate",
            request_params={"steps": 8},
        )
        self.assertEqual(j2.full_path, "mengsk/jyby-mv/video:shot-b01/job_final_02")
        self.store.update_task_status(j2.task_id, "completed", duration_seconds=36.5)

        # 获取该 Asset 下的所有 Job 生成历史
        history = self.store.list_asset_jobs(user, proj, asset)
        self.assertEqual(len(history), 2)
        job_ids = [t.job_id for t in history]
        self.assertIn("job_draft_01", job_ids)
        self.assertIn("job_final_02", job_ids)


if __name__ == "__main__":
    unittest.main()
