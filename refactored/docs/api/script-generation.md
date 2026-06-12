# Script Generation API

## POST /api/script/generate-with-images

Generate a video script from text and optional images. Creates a processing job.

### Request body

```json
{
  "title": "My Video Title",
  "scenes": [
    {
      "text": "Scene description",
      "images": ["https://example.com/image1.jpg"],
      "duration": 5,
      "voiceover": "Optional voiceover text"
    }
  ],
  "style": "cinematic",
  "voice": "default"
}
```

### Response

```json
{
  "job_id": "job_xyz789",
  "status": "queued",
  "message": "Script generation started"
}
```

## GET /api/script/jobs/:job_id

Get job status and partial result.

### Response

```json
{
  "job_id": "job_xyz789",
  "status": "PROCESSING",
  "progress": 0.45,
  "current_step": "scene_3_of_5"
}
```

## GET /api/script/jobs/:job_id/full

Get job status with full result data.

## GET /api/script/:script_id

Retrieve a completed script by ID.

---

Also available at `/api/v1/script/...` (same endpoints).
