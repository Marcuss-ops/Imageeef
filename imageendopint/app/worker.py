from __future__ import annotations

import asyncio
import json
import logging
import shutil
import time
from pathlib import Path

import httpx

from .browser import run_generation
from .config import load_settings, Settings
from .models import JobRecord
from .store import RedisJobStore

logger = logging.getLogger("image_endpoint.worker")

# Global semaphore to limit concurrent browser instances
job_semaphore: asyncio.Semaphore | None = None


def _job_logger(job_id: str, out_dir: Path) -> tuple[logging.LoggerAdapter, logging.Handler]:
    out_dir.mkdir(parents=True, exist_ok=True)
    file_handler = logging.FileHandler(out_dir / "job.log", encoding="utf-8")
    file_handler.setFormatter(logging.Formatter("%(asctime)s %(levelname)s %(name)s %(message)s"))

    job_logger = logging.getLogger(f"image_endpoint.worker.job.{job_id}")
    job_logger.setLevel(logging.INFO)
    job_logger.handlers = [file_handler]
    job_logger.propagate = True
    return logging.LoggerAdapter(job_logger, {"job_id": job_id}), file_handler


async def _notify_webhook(job: JobRecord, out_dir: Path) -> None:
    if not job.request.webhook_url:
        return

    logger.info("job_id=%s sending webhook to %s", job.id, job.request.webhook_url)
    try:
        async with httpx.AsyncClient(timeout=60.0) as client:
            files = {}
            if job.status == "succeeded" and job.result and "images" in job.result:
                for img_name in job.result["images"]:
                    img_path = out_dir / img_name
                    if img_path.exists():
                        files[img_name] = (img_name, img_path.read_bytes(), "image/jpeg")

            # Send job data + files as multipart/form-data
            data = {"job_json": json.dumps(job.to_dict())}
            response = await client.post(job.request.webhook_url, data=data, files=files)
            logger.info("job_id=%s webhook response status=%d", job.id, response.status_code)
    except Exception as exc:
        logger.error("job_id=%s webhook failed: %s", job.id, exc)


async def _run_once(store: RedisJobStore, settings: Settings, job_id: str) -> None:
    job = await store.get_job(job_id)
    if job is None:
        logger.warning("job_id=%s not found in redis", job_id)
        return

    if job.status != "queued":
        logger.warning("job_id=%s skipped because status=%s", job_id, job.status)
        return

    out_dir = Path("outputs") / job.id
    job_log, file_handler = _job_logger(job.id, out_dir)
    job.mark_running()
    await store.update_job(job)

    try:
        job_log.info(
            "job received project_id=%s client_ip=%s user_agent=%s",
            job.request.project_id,
            job.client_ip or "-",
            job.user_agent or "-",
        )
        result = await asyncio.wait_for(
            run_generation(settings, job.request, out_dir),
            timeout=settings.job_timeout_seconds,
        )
        job.mark_succeeded(result)
        job_log.info("job succeeded")
    except Exception as exc:  # noqa: BLE001
        job.mark_failed(f"{type(exc).__name__}: {exc}")
        job_log.exception("job failed: %s", exc)
    finally:
        await store.update_job(job)
        # Notify webhook if configured
        await _notify_webhook(job, out_dir)
        file_handler.close()


async def _cleanup_old_jobs(settings: Settings) -> None:
    """Periodically remove job output directories older than JOB_RETENTION_HOURS."""
    outputs_dir = Path("outputs")
    if not outputs_dir.exists():
        return

    retention_seconds = settings.job_retention_hours * 3600
    now = time.time()
    
    logger.info("Starting automated cleanup (retention: %dh)", settings.job_retention_hours)
    
    deleted_count = 0
    for item in outputs_dir.iterdir():
        # Check if it's a UUID-like directory (job output)
        if item.is_dir() and not item.name.startswith(".") and item.name != "Log" and item.name != "webhook-received":
            # Check directory age
            mtime = item.stat().st_mtime
            if (now - mtime) > retention_seconds:
                try:
                    logger.info("Deleting old job directory: %s", item.name)
                    shutil.rmtree(item)
                    deleted_count += 1
                except Exception as e:
                    logger.warning("Failed to delete %s: %s", item.name, e)
    
    if deleted_count > 0:
        logger.info("Cleanup finished: deleted %d old job directories", deleted_count)


async def _run_cleanup_loop(settings: Settings) -> None:
    """Run the cleanup task once an hour."""
    while True:
        try:
            await _cleanup_old_jobs(settings)
        except Exception as e:
            logger.error("Error in cleanup task: %s", e)
        await asyncio.sleep(3600) # Once an hour


async def run_worker() -> None:
    global job_semaphore
    settings = load_settings()
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
    )
    
    job_semaphore = asyncio.Semaphore(settings.max_concurrent_jobs)
    
    store = RedisJobStore(
        redis_url=settings.redis_url,
        queue_name=settings.redis_queue_name,
        job_key_prefix=settings.redis_job_key_prefix,
    )

    logger.info("Worker started (concurrency limit: %d), waiting for jobs...", settings.max_concurrent_jobs)
    
    # Start cleanup background task
    asyncio.create_task(_run_cleanup_loop(settings))
    
    try:
        while True:
            job_id = await store.pop_next_job_id(timeout_seconds=5)
            if job_id is None:
                continue
            logger.info("dequeued job_id=%s", job_id)
            
            # Use semaphore to limit concurrency
            async def wrapped_job(jid):
                async with job_semaphore:
                    await _run_once(store, settings, jid)
            
            asyncio.create_task(wrapped_job(job_id))
    finally:
        await store.close()


def main() -> None:
    asyncio.run(run_worker())


if __name__ == "__main__":
    main()
