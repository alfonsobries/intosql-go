# Changelog

## 0.3.2

- The README says MariaDB and SQLite dumps, and SQLite database files, convert to any of the six databases.

## 0.3.1

- The README says a SQL Server script converts to any of the six databases, and its report carries the codes of each target.

## 0.3.0

- `ObjectMaterializedView`, `ObjectRule`, `ObjectExtension`, `ObjectPolicy`, `ObjectType` and `ObjectOther`: what PostgreSQL dumps leave out.

## 0.2.1

- The README says a MySQL or MariaDB dump converts to any of the six databases, and its report carries the codes of each target.

## 0.2.0

- Reports of database dumps: `Source`, `Tables` and `NotConverted`.

## 0.1.0

- `Convert`, `ConvertFile` and `ConvertURL`: create, upload, start and wait in one call.
- Every conversion endpoint: `Create`, `CreateFromURL`, `Upload`, `Start`, `Retrieve`, `List`, `Cancel`, `CreateCase`, `Delete` and `Wait`; `Usage`.
- `NewSpec` for a conversion spec with IntoSQL's defaults.
- `VerifyWebhook` and `ParseWebhook`, which returns the typed `WebhookEvent`.
- Typed errors: `*Error` with `Status`, `Code`, `Fields` and `RequestID`; `*ConversionFailedError`, `*ConversionCanceledError` and `*WaitTimeoutError`.
- Retries on `rate_limited`, `too_many_active`, `request_in_progress`, 408, 425, 5xx and lost connections, with the same idempotency key; never `already_started`.
