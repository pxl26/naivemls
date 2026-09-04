# Naive MLS

This repository contains three independent Go modules:

```text
naivemls/
├── e2ee-service/     MLS epoch and commit storage
├── message-service/  encrypted-message storage and synchronization
├── client-sdk/       client-side MLS code and browser-facing HTTP API
└── web/              global-room chat UI
```

The root `go.work` file connects the modules for local development. None of the modules imports another module.

## Run the services

Open three terminals at the repository root:

```sh
go run ./e2ee-service
go run ./message-service
go run ./client-sdk/cmd/client-sdk
```

- Client SDK: `http://localhost:8080`
- E2EE service: `http://localhost:8081`
- Message service: `http://localhost:8082`
- State files: `./data/e2ee.txt` and `./data/messages.txt`

Configuration:

```sh
go run ./e2ee-service -addr=:8081 -data=./data/e2ee.txt
go run ./message-service -addr=:8082 -data=./data/messages.txt -e2ee-url=http://127.0.0.1:8081
go run ./client-sdk/cmd/client-sdk -addr=:8080 -e2ee-url=http://127.0.0.1:8081 -message-url=http://127.0.0.1:8082
```

Run the web UI:

```sh
cd web
npm run dev
```

Then open `http://localhost:3000`. The UI synchronizes automatically every 2.5 seconds and remembers the display name, local epoch, and Client SDK URL in browser storage.

## Client SDK

```go
package main

import (
	"context"

	mls "github.com/pxl26/naivemls/client-sdk"
)

func example() error {
	client := mls.NewClient("http://localhost:8081", "http://localhost:8082")
	ctx := context.Background()

	if _, err := client.SubmitProposal(ctx, 1, []byte("serialized MLS proposal")); err != nil {
		return err
	}
	if _, err := client.SendMessage(ctx, 1, "alice", []byte("encrypted MLS application message")); err != nil {
		return err
	}
	return nil
}
```

## API

Timestamps are Unix milliseconds. Time ranges are inclusive.

### Upload an MLS proposal

```http
POST /e2ee/v1/global-room/proposal
Content-Type: application/json

{
  "epoch": 1,
  "data": "BASE64_ENCODED_MLS_PROPOSAL"
}
```

The proposal is accepted only when `epoch == current_epoch + 1`. An accepted proposal is the commit for that epoch.

### Sync MLS commits

```http
GET /e2ee/v1/global-room/sync?local_epoch=0
```

```json
{
  "current_epoch": 1,
  "commits": [
    {"epoch": 1, "data": "BASE64_ENCODED_MLS_PROPOSAL"}
  ]
}
```

The response contains commits from `local_epoch + 1` through `current_epoch`.

### Send an encrypted message

```http
POST /message/v1/global-room/message
Content-Type: application/json

{
  "epoch": 1,
  "sender": "alice",
  "data": "BASE64_ENCODED_CIPHERTEXT"
}
```

The message service asks the E2EE service for the current epoch. It stores the ciphertext only when the supplied epoch is current.

### Sync encrypted messages

```http
GET /message/v1/global-room/message/sync?from=0&to=999
```

```json
{
  "messages": [
    {
      "id": 1,
      "epoch": 1,
      "sender": "alice",
      "data": "BASE64_ENCODED_CIPHERTEXT",
      "timestamp": 999
    }
  ]
}
```

Epoch mismatches return HTTP `409` with the server's `current_epoch`. If the E2EE service cannot be reached, sending a message returns HTTP `502` and the message is not stored. The client SDK returns non-success responses as `*mls.APIError`.

## Browser-facing Client SDK API

### `sendMessage`

```http
POST /client-sdk/v1/sendMessage
Content-Type: application/json

{
  "epoch": 0,
  "sender": "alice",
  "message": "Hello"
}
```

### `syncMessage`

```http
GET /client-sdk/v1/syncMessage?from=0&to=9999999999999
```

The Client SDK uses a `MessageCodec` interface before calling message-service. Its runnable command currently uses `PassthroughCodec`, which is intentionally not encryption. Replace it with the MLS application-message codec before production; the web API does not need to change.

## Test

Run every module from the repository root:

```sh
go test ./client-sdk/... ./e2ee-service/... ./message-service/...
cd web && npm run build
```
