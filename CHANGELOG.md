# Changelog

## 0.1.0

- `Convert`, `ConvertFile` and `ConvertURL`: create, upload, start and wait in one call.
- Every conversion endpoint: `Create`, `CreateFromURL`, `Upload`, `Start`, `Retrieve`, `List`, `Cancel`, `CreateCase`, `Delete` and `Wait`; `Usage`.
- `NewSpec` for a conversion spec with IntoSQL's defaults.
- `VerifyWebhook` and `ParseWebhook`, which returns the typed `WebhookEvent`.
- Typed errors: `*Error` with `Status`, `Code`, `Fields` and `RequestID`; `*ConversionFailedError`, `*ConversionCanceledError` and `*WaitTimeoutError`.
- Retries on `rate_limited`, `too_many_active`, `request_in_progress`, 408, 425, 5xx and lost connections, with the same idempotency key; never `already_started`.
