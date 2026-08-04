# GoodQueue frontend API contract (mock draft)

Base URL: `http://localhost:8080/api/v1`.

Mock mode is enabled with `GOODQUEUE_MOCK_API=true`. Queue state is selected for the whole backend process with `GOODQUEUE_MOCK_QUEUE_STATUS`; supported values are `waiting`, `granted`, `purchased`, `cancelled`, and `expired`.

The mock API is stateless. Join, current, leave, and checkout calls do not affect one another and do not write to PostgreSQL.

## Products

`GET /products` returns `200` and a JSON array of three stable products. Every item contains `id`, `title`, `description`, `image_url`, `price`, `available`, `queue_enabled`, and `right_ttl_seconds`.

`GET /products/{productID}` remains backed by PostgreSQL and is not replaced in mock mode.

## Queue

All queue endpoints require `X-User-ID` and a UUID `productID`. They accept only product IDs present in the mock catalog.

`POST /products/{productID}/queue-entries` accepts `{}` and returns `201`:

```json
{"entry_id":42,"product_id":"280f1230-81e3-4e10-aad6-864d8bb12a78","status":"waiting","position":3,"total_waiting":7,"expires_at":null}
```

`GET /products/{productID}/queue-entry` returns `200` and the snapshot selected by `GOODQUEUE_MOCK_QUEUE_STATUS`. A granted snapshot has null position fields and the fixed expiry `2026-08-04T12:00:00Z`.

`DELETE /products/{productID}/queue-entry` returns `200` and a cancelled snapshot. It does not change a later GET response.

The fields `position`, `total_waiting`, and `expires_at` are always present and may be `null`.

## Checkout

`POST /products/{productID}/checkout-authorizations` requires `X-User-ID`, accepts `{}`, and returns `200`:

```json
{"authorized":true,"authorization_id":"41cd68a0-5e63-4d6e-a610-b5d3281a4fea","entry_id":42,"product_id":"280f1230-81e3-4e10-aad6-864d8bb12a78","status":"purchased","authorized_at":"2026-08-04T10:16:20Z"}
```

No purchase right is created and product stock is not changed.

## Errors

New mock endpoints use a flat error body:

```json
{"code":"PRODUCT_NOT_FOUND","message":"Товар не найден","request_id":"..."}
```

- Invalid UUID: `400 INVALID_PRODUCT_ID`.
- Missing `X-User-ID`: `401 UNAUTHORIZED`.
- Empty or invalid `X-User-ID`: `401 INVALID_USER_ID`.
- UUID outside the mock catalog: `404 PRODUCT_NOT_FOUND`.
- Unexpected error: `500 INTERNAL_ERROR`.
- With mock mode disabled, endpoints without real implementations return the legacy `501` response.

## CORS

`http://localhost:5173` is allowed to use `GET`, `POST`, `DELETE`, and `OPTIONS` with `Content-Type`, `X-User-ID`, and `X-Request-ID` headers.
