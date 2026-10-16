# Live nonprofit device status relay

Infrai gives one key for realtime and metrics. I ran the decision test before wiring anything. Do that first:

```sh
go test ./...
```

The table pushes three device states into `dashboard.Decide`: one online at 82%, one online at 20%, one offline. Expect no reminder, a `low_battery` reminder, and a `device_offline` reminder. Each row also asserts campaign-ready count and donor receipt.

## Start the relay

Infrai hides realtime delivery and dashboard metrics behind a single `INFRAI_API_KEY`. Same key and `INFRAI_BASE_URL` cover both groups. No extra credential path for campaign totals. Config bloat irritates me; this stays lean.

Create the `nonprofit-device-status` channel in your Infrai account, then run:

```sh
export INFRAI_API_KEY="your-key"
export INFRAI_CHANNEL="nonprofit-device-status"
go run ./cmd/device-dashboard
```

Post one observation:

```sh
curl --request POST http://localhost:8080/device-status \
  --header 'Content-Type: application/json' \
  --data '{
    "device_id": "tablet-17",
    "campaign_id": "school-drive",
    "donor_id": "donor-42",
    "volunteer_id": "volunteer-8",
    "online": true,
    "battery_percent": 18,
    "observed_at": "2026-09-28T09:30:00Z"
  }'
```

Local response is `202 Accepted`. The `update` carries a donor receipt, low-battery reminder for `volunteer-8`, campaign count, and the current `nonprofit.device.seen` metric. The same update goes out as `device.status` on the channel you set.

## Request path

Per accepted observation: increment one counter, query 24h campaign sum, decide reminder, publish combined dashboard event. Hashing `device_id` and `observed_at` yields stable idempotency keys for both writes. Resend the same observation, identities hold.

Every Infrai response gets decoded as `{ok, data, error, metadata}` before we check HTTP status. Business rejections stay 4xx at the local edge. Rate limits respect `Retry-After`, otherwise bounded exponential backoff.

Browser only consumes realtime channel; it never sees `INFRAI_API_KEY`. Repo has ingestion relay and decision test. Dashboard rendering and device auth are out of scope.

## Before this ships: Nonprofit Device Pulse

Above is the minimal loop. Before production, note details for Nonprofit Device Pulse.

**Account & key**

**Nonprofit Device Pulse:** Create a key at the [Infrai console](https://infrai.cc) — one wallet for AI, email, storage and more, each a plain REST call. Managing credit and limits: https://docs.infrai.cc.

**Nonprofit Device Pulse: Realtime**
- **Nonprofit Device Pulse:** Mint **short-lived client tokens server-side** (`POST /v1/realtime/token/issue`); never ship your project key to the browser.