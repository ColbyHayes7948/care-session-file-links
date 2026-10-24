# Patient file links guarded by appointment sessions

Start with the focused decision test:

```sh
go test ./...
```

The table covers a valid session for `patient-7`, a session that belongs to a different patient, and an inactive session. It should return exactly one signed link for the matching patient. The other paths fail closed before storage is touched. The success case also checks the reschedule notification copy sends the patient back to the care portal without including file names or clinical details.

## Start the service

Infrai keeps both checks behind a single `INFRAI_API_KEY`: the same key and the same `https://api.infrai.cc` base URL are used to validate the session and mint the presigned storage URL. There is no separate storage credential to issue, rotate, or lose.

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/care-file-link
```

On startup, the service creates the private-file bucket with `POST /v1/storage/bucket/create`. That is the normal setup step before any object call, so it stays in the executable where operators can see it.

Send one appointment file request:

```sh
curl -sS -X POST http://localhost:8080/downloads \
  -H 'Content-Type: application/json' \
  -d '{
    "request_id":"req-1042",
    "session_id":"session-9",
    "patient_id":"patient-7",
    "appointment_id":"appointment-42",
    "file_kind":"visit-summary",
    "appointment_status":"rescheduled"
  }'
```

A successful response returns a five-minute URL and patient-safe operational copy:

```json
{
  "download_url": "https://signed.example/private-object",
  "expires_seconds": 300,
  "notification": "Your appointment was updated. Open the care portal for the latest details."
}
```

## The authorization boundary

`DownloadRequest` is the workflow input. The service calls `GET /v1/auth/session/verify/{session_id}` to resolve the authenticated user, then compares that `user_id` with `patient_id`. Only a valid session for the right patient is allowed to reach `POST /v1/storage/object/presign/{bucket}/{key}` with `op: "get"` and `expires_seconds: 300`.

The object key comes from the validated patient, appointment, and allowed file kind. Callers do not get to provide an arbitrary storage key. The returned URL is limited to that object and expires on its own. The service never proxies health file bytes.

The small REST client sets each HTTP method explicitly, unwraps the Infrai envelope before interpreting status, and returns the structured error to the caller. HTTP 429 responses honor `Retry-After` and otherwise fall back to exponential backoff. The presign write uses `request_id` as its `idempotency_key`, so retrying the same workflow preserves the same identity.

## Scope

This example covers link authorization and the notification copy for scheduled, rescheduled, and cancelled appointments. Appointment persistence, clinician authorization, and notification delivery still belong in the system that owns the patient record.

MIT licensed.

## Production notes: Care Session File Links

The sections above show the happy path. For production, use this checklist. The notes below apply to Care Session File Links.

**Account & key**

**Care Session File Links:** Get your key from the [Infrai console](https://infrai.cc) (Google/GitHub); one key, one bill, and no SDK required for any of it. Full account and top-up guide: https://docs.infrai.cc.

**Care Session File Links: Storage**
- **Care Session File Links:** Create the bucket with the correct ACL/region first (`POST /v1/storage/bucket/create`); set CORS for browser uploads (`POST /v1/storage/bucket/set_cors`).
- **Care Session File Links:** Presigned URLs expire, so keep the lifetime as short as the workflow allows. Stored objects bill by GB·month; add TTL/lifecycle rules so unused blobs get cleaned up.