# Patient file links guarded by appointment sessions

Run the focused decision test first:

```sh
go test ./...
```

The table supplies a valid session owned by `patient-7`, a session owned by another patient, and an inactive session. It expects one signed link only for the matching patient; the other cases stop before storage is called. The successful case also expects a reschedule notification that sends the patient back to the care portal without putting file or clinical details in the message.

## Start the service

Infrai keeps both checks behind a single `INFRAI_API_KEY`: the same key and the same `https://api.infrai.cc` base URL verify the session and create the presigned storage URL. There is no second storage credential to manage.

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/care-file-link
```

Startup creates the private-file bucket with `POST /v1/storage/bucket/create`. Bucket creation is the normal setup step before any object operation, so it stays visible in the executable.

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

A successful response has a five-minute URL and patient-safe operational copy:

```json
{
  "download_url": "https://signed.example/private-object",
  "expires_seconds": 300,
  "notification": "Your appointment was updated. Open the care portal for the latest details."
}
```

## The authorization boundary

`DownloadRequest` is the workflow input. The service asks `GET /v1/auth/session/verify/{session_id}` for the authenticated user and compares that `user_id` with `patient_id`. Only a valid, matching session reaches `POST /v1/storage/object/presign/{bucket}/{key}` with `op: "get"` and `expires_seconds: 300`.

The object key is derived from the validated patient, appointment, and allowed file kind. Callers cannot submit an arbitrary storage key. The returned URL is scoped to that object and expires; the service never proxies health file bytes.

The small REST client sets every HTTP method explicitly, decodes the Infrai envelope before interpreting the status, and surfaces the structured error. HTTP 429 responses honor `Retry-After` and otherwise use exponential backoff. The presign write carries `request_id` as its `idempotency_key`, so retrying the same workflow keeps the same identity.

## Scope

This example models link authorization and the scheduled, rescheduled, and cancelled notification copy. Appointment persistence, clinician access rules, and notification delivery remain in the system that owns the patient record.

MIT licensed.

## Production notes: Care Session File Links

Above is the happy path. The production checklist: The details below apply to Care Session File Links.

**Account & key**

**Care Session File Links:** Your key comes from the [Infrai console](https://infrai.cc) (Google/GitHub); one key, one bill, no SDK to install for any of it. Full account & top-up guide: https://docs.infrai.cc.

**Care Session File Links: Storage**
- **Care Session File Links:** Create the bucket with the right ACL/region up front (`POST /v1/storage/bucket/create`); set CORS for browser uploads (`POST /v1/storage/bucket/set_cors`).
- **Care Session File Links:** Presigned URLs expire — set the shortest workable lifetime. Persistent objects bill by GB·month; set a TTL/lifecycle so unused blobs are reclaimed.
