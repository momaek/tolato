# Patato integration

Tolato provides machine inventory, an activity feed, local machine proofs and
opt-in patato-node supervision. Patato keeps a **readonly** API key for inventory,
activity and proof verification. Operators use a separate **writable** key for
service changes. All endpoints authenticate the key's live owner and enforce
that owner's node grants.

## Inventory

`GET /api/v1/nodes` still returns an array, now including every visible node
without the former 200-node limit. Both this endpoint and `GET /api/v1/nodes/:id`
populate `country_code`, `city` and `asn` from stored GeoIP data. Unavailable
GeoIP fields are omitted. Optional `?name=` (case-insensitive exact hostname or
alias) and `?ip=` (exact stored IP) filters apply inside the visibility scope;
combined filters use AND. An invisible match produces an empty array.

The CLI preserves these fields in `tolato nodes list --json` and
`tolato nodes get --json <node>`.

## Activity feed

`GET /api/v1/nodes/:id/commands` requires viewer access and accepts readonly keys.
It includes all audit sources, including terminal, file and service activity.
Each item contains only `id` (number), `command`, `source`, `actor`, `created_at`.
Output, output digests, API key identifiers and metadata are never selected or
returned by this endpoint, even for writable keys.

Parameters:

| Parameter | Meaning |
| --- | --- |
| `since` | Optional RFC 3339 timestamp, including fractional seconds; strict `created_at > since` |
| `page_size` | 1–100, default 20 |
| `page` | Positive offset page, default 1; use cursors for polling |
| `cursor` | Opaque `next_cursor` or `checkpoint`; use without `since` or `page` |

Results are ordered by `(created_at ASC, id ASC)`. Responses retain the existing
`items`, `total`, `page`, `page_size`, `total_pages` shape and add `next_cursor`
and `checkpoint`. Invalid timestamps, pagination or cursors return 400.

```json
{
  "items": [{"id": 42, "command": "uptime", "source": "cli", "actor": "alice", "created_at": "2026-10-31T08:00:00Z"}],
  "total": 1, "page": 1, "page_size": 100, "total_pages": 1,
  "next_cursor": "", "checkpoint": "<opaque-token>"
}
```

Polling algorithm:

1. Start with `?since=<start>&page_size=100` (omit `since` for full history).
2. Process a page, then follow `?cursor=<next_cursor>&page_size=100` until
   `next_cursor` is empty. Persist the resulting `checkpoint` only after all
   events were processed. Keep event IDs for idempotent processing after retries.
3. On the next poll, pass that checkpoint as `cursor`. It starts a new window
   above the previous insertion ID. Equal or older timestamps on subsequently
   inserted events do not get lost by advancing a timestamp alone. The original
   `since` filter remains in force. An empty poll also returns a checkpoint.

A page cursor binds the node, timestamp filter, insertion-ID upper bound and
last `(created_at,id)` position, so normal appends cannot shift its pages. A
checkpoint is a committed insertion watermark: the audit writer and watermark
reader coordinate with per-node PostgreSQL transaction advisory locks, including
across server instances. A later ID cannot overtake an earlier uncommitted audit
write for that node. A watermark times out after five seconds rather than
advancing past a blocked write. Bulk imports that write the audit table directly
must use the same writer contract or be followed by a rescan with ID deduplication.
Do not delete unread audit rows during traversal. Offset `page` is provided for
simple browsing; its requests do not share a frozen window.

```bash
tolato commands panel-host --since 2026-10-31T00:00:00Z --page-size 100
tolato commands panel-host --cursor '<next_cursor-or-checkpoint>'
```

Existing Patato clients that use `items` and numeric IDs can read the endpoint
immediately. For reliable multipage polling they should drain `next_cursor` and
persist `checkpoint` instead of advancing `since` after only the first page.

## Local machine proofs

Enable a root-only Unix socket on the agent:

```bash
tolato-agent --server wss://tolato.example/ws/agent \
  --data-dir /var/lib/tolato \
  --attestation.socket /var/lib/tolato/attestation/agent.sock
```

The installer also accepts `--attestation.enabled`, choosing the path above
under `TOLATO_DATA_DIR`. The feature is disabled by default. It requires root,
a root-owned private socket directory (0700), and safe root-owned ancestors.
The socket has mode 0600. Non-root agents fail explicitly when enabling it.
The listener refuses regular files, symlinks and live sockets at its path;
an abandoned socket from a crashed instance can be removed on startup.

The relying party (Patato) first issues a cryptographically random challenge
(at least 128 bits), tied to a pending enrollment and expected Tolato node ID.
patato-node then asks its local agent for a proof:

```bash
curl --unix-socket /var/lib/tolato/attestation/agent.sock \
  -H 'Content-Type: application/json' \
  -d '{"audience":"patato","nonce":"<challenge-from-patato>"}' \
  http://localhost/v1/attestations
```

The response is `{"token":"ta1.<payload>.<signature>"}`. Nothing is signed until
the agent has a persisted node identity. Tokens live for 60 seconds. Audience
is 1–128 bytes and nonce 16–256 bytes. Responses must not be cached.

Patato verifies with its readonly key over HTTPS:

```http
POST /api/v1/attestations/verify
Authorization: Bearer <readonly-key>
Content-Type: application/json

{"token":"ta1.…","audience":"patato","nonce":"<expected-challenge>"}
```

Success returns `{"valid":true,"claims":{"node_id":"…","audience":"patato",
"nonce":"…","issued_at":1793448000,"expires_at":1793448060}}`. Times are Unix
seconds. Invalid, expired, tampered, wrong-audience or wrong-nonce proofs return
400. Invisible or deleted nodes return 404. The verification key needs viewer
access to the proven node. Rotating the agent secret invalidates prior proofs;
offline agents may still prove their persisted identity during the token's TTL.

**Enrollment belongs to Patato:** it must compare the verified node ID to the
pending enrollment, bind the audience to its own configured identifier, and
atomically consume the challenge once before issuing a machine token. A valid
signature by itself is not a reusable enrollment credential. Tolato does not
store or globally consume these nonces. Root on a machine controls its identity.

### Wire format

This is a versioned HMAC protocol, not JWT:

- `payload`: unpadded base64url of UTF-8 JSON with the five claims above.
- Signing key: `HMAC-SHA256(agent_secret, "tolato/attestation/v1")`.
- Signature: `HMAC-SHA256(signing_key, "ta1." + payload)`, unpadded base64url.
- Verification checks the current node secret, constant-time signature equality,
  exact expected audience/nonce, expiry, TTL <=60s and issue time <=now+5s.

[A shared test vector](fixtures/attestation-v1.json) pins agent/server compatibility.
The signing key never leaves the agent/server. Consumers use the verification API.

## Managed patato-node

On a **Linux root agent**, enable `--managed-services.enabled` (also accepted by
the installer). This advertises service protocol version 1. Older or disabled
agents return 501 immediately; offline nodes return 503.

Tolato runs `patato-node run -c <private-config>` directly, without a shell or an
additional systemd service. The agent restarts a crashed child with exponential
backoff (1–32 seconds), retains desired state across its own restart, and stops
the child before self-update/exec. Linux parent-death signaling and process-group
termination prevent orphan children. stdout/stderr from the child are discarded;
the API exposes running state, PID, digest, restart count and a generic error.

| Endpoint | Permission |
| --- | --- |
| `GET /api/v1/nodes/:id/services/patato-node` | readonly key + viewer |
| `POST /api/v1/nodes/:id/services/patato-node` | writable key + operator |

POST body:

```json
{
  "action": "install",
  "binary_url": "https://github.com/momaek/patato/releases/download/<tag>/patato-node-linux-amd64",
  "sha256": "<64-lowercase-hex-digest-from-the-release>",
  "config": {"panel":"https://patato.example","token":"<machine-token>","machine":"tokyo-01"}
}
```

Actions are `install`, `upgrade`, `restart`, `stop`. Install/upgrade require an
HTTPS binary URL (no embedded credentials or HTTP redirect downgrade) and a
matching SHA-256. Downloads are capped at 256 MiB with a two-minute deadline.
Config must be a JSON object (valid YAML for patato-node), at most 64 KiB, with
an HTTPS `panel` and a `token`; additional Patato configuration fields are kept.
It is required on first install and may be omitted on upgrade to retain the
enrollment and settings. Choose the binary for the target node's architecture.

Configuration and the manifest are atomically written with mode 0600 in
`<data-dir>/services/patato-node`, and executables use mode 0700. Digest failure
leaves the existing process running. A new executable that fails to spawn rolls
back to the previous binary and configuration. There is no application-level
health probe: a process that starts and later exits enters supervised retry.
Stopped services stay stopped after an agent restart. `restart` also starts an
installed, stopped service. Stop before transferring ownership to another manager.

First-time enrollment still uses Patato's existing enroll flow, or its future
proof-based flow described above, to obtain a machine token. This Tolato change
does not modify Patato's enrollment endpoints. When adopting an existing
systemd-managed installation, disable and stop its `patato-node` unit once before
enabling supervision; do not run both managers for the same node.

```bash
# config.json contains the Patato configuration and should be mode 0600.
tolato services install tokyo-01 --binary-url 'https://…/patato-node-linux-amd64' \
  --sha256 '<release-digest>' --config ./config.json
tolato services upgrade tokyo-01 --binary-url 'https://…/patato-node-linux-amd64' \
  --sha256 '<new-release-digest>'
tolato services status tokyo-01
tolato services restart tokyo-01
tolato services stop tokyo-01
```

Mutations record a `service:<action> patato-node` audit event before being sent to
the agent. A failed audit write prevents the mutation. Config, tokens, download
URLs and child output are never written to that event or returned by status.
The audit event records the attempt; use service status to check the outcome.
