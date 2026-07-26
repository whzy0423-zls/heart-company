# Free Video Workflow Design

## Goal

Run the guided project workflow from script through generation, explicit version selection, composition, preview, and export without making any paid video-provider request.

## Safety Boundary

Video generation is fail-closed. `VIDEO_GENERATION_MODE` defaults to `demo`. Paid generation is enabled only when both conditions are true:

- `VIDEO_GENERATION_MODE=paid`
- `VIDEO_PAID_GENERATION_ACK=ALLOW_PAID_VIDEO_GENERATION`

Any missing, misspelled, or partial configuration remains in demo mode. Database-backed model settings may change the provider URL, key, and model, but cannot change the effective generation mode. All generation routes, including legacy, safe single-shot, and safe batch routes, pass through the same server-side mode gate.

## Demo Generation

The video store owns a small demo renderer abstraction. Its production implementation invokes local FFmpeg with a generated color source and produces a valid H.264 MP4 using the requested aspect ratio and duration. It does not make an HTTP request or use provider credentials.

The rendered file is uploaded through the existing object uploader. With no OSS configuration, the existing local uploader writes under `UPLOAD_DIR` and returns an authenticated `/api/uploads/...` URL. The generation row uses provider `demo`, a deterministic request-key task identifier, terminal status `completed`, and the real local video URL and metadata.

Safe request-key behavior remains idempotent. A prepared submission is rendered before entering the submitting state. Successful demo persistence follows the existing accepted-to-completed linkage. A demo-only cancellation path releases the submission lock on proven local failure; it is not available to paid gateway failures and therefore does not weaken ambiguous-outcome protection.

Reference assets do not need public OSS URLs in demo mode because no external gateway fetches them. Paid mode retains the existing public-URL validation.

## Composition

The composer accepts the existing HTTP URLs plus local `/api/uploads/...` URLs. Local URLs are resolved beneath the configured upload root using traversal-safe path checks and copied directly to the composition workspace. This avoids an authenticated HTTP loopback request while keeping the browser-facing URL unchanged.

FFmpeg then performs the existing real concat/transition workflow. The composed MP4 is uploaded through the same uploader and stored on the project, so preview and download use the actual generated artifact.

## Workflow API And UI

The workflow response includes `generationMode: "demo" | "paid"`. The guided generation step shows a persistent status banner:

- Demo: `免费演练模式：使用本地占位视频，不会调用收费接口。`
- Paid: an explicit paid-mode warning before generation actions.

The default is demo and the UI does not provide a client-side switch. Enabling paid mode remains an operator-level server configuration decision.

## Failure Handling

- Missing FFmpeg: generation fails with an actionable local-renderer error and no provider call.
- Local upload failure: the prepared demo submission is cancelled and its activity lock is released.
- Local source path escape or missing file: composition fails without reading outside `UPLOAD_DIR`.
- Paid mode without the exact acknowledgement: effective mode remains demo.
- Existing paid unknown-outcome and reconciliation behavior is unchanged.

## Verification

Tests must prove:

1. Default and malformed configurations resolve to demo; only the two-factor configuration resolves to paid.
2. Model-config reload preserves demo mode.
3. Demo single and batch generation make zero gateway calls, create valid completed versions, and reuse a request key.
4. Paid mode still uses exactly one provider POST under the existing safety state machine.
5. Local upload URLs compose into a real MP4 without HTTP loopback.
6. The workflow API exposes the mode and the UI renders the no-charge banner.
7. A full browser flow reaches current final video while a network guard rejects any provider URL.
