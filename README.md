# biometric-matcher

A fingerprint enrollment/verification/identification system split across two
services on purpose:

- **java-matcher** — stateless gRPC service wrapping
  [SourceAFIS](https://sourceafis.machinezoo.com/) for minutiae extraction
  and matching. Never touches a database, never sees a name or DOB, only
  raw scan bytes and templates.
- **go-client** — CLI + eventual API gateway. Owns persistence, encryption
  of stored templates, and all biographic data. Calls java-matcher over
  gRPC for anything biometric.

The split exists so the matching engine can be swapped, scaled, or
re-benchmarked (e.g. against NIST NBIS) without touching how identities
are stored or exposed. See `proto/biometric.proto` for the full contract
and the reasoning behind each RPC.

## Layout

```
proto/                  shared source of truth for the gRPC contract
java-matcher/            SourceAFIS-backed matcher service
go-client/                CLI and generated Go client stubs
```

`proto/biometric.proto` is the canonical copy. The copy under
`java-matcher/src/main/proto/` exists only because Gradle's protobuf
plugin expects proto files inside the module. If the contract changes,
edit `proto/biometric.proto` and re-copy, don't edit both independently.

## Getting both sides running

### 1. Generate code from the proto

```
make proto
```

This generates the Go client/server stubs into `go-client/gen/biometricpb`
and lets Gradle's protobuf plugin handle Java codegen automatically on
build. Requires `protoc` and the Go gRPC plugins locally, see the Makefile
for the exact install commands if you don't have them.

### 2. Java matcher

```
cd java-matcher
./gradlew run
```

Add the SourceAFIS dependency in `build.gradle.kts` (already declared,
just needs Maven Central reachable) before this will actually compile.
`MatcherServiceImpl.java` has the enroll/verify/identify wiring stubbed
with TODOs at the exact lines where SourceAFIS calls go, since the exact
API surface is worth double checking against current SourceAFIS docs
rather than trusting this from memory.

### 3. Go CLI

```
cd go-client
go run ./cmd/biometric-cli enroll --scan ./testdata/sample.iso --out ./testdata/template.bin
go run ./cmd/biometric-cli verify --scan ./testdata/sample.iso --template ./testdata/template.bin
```

## What's deliberately not built yet

- Template encryption at rest (planned to live in `go-client/internal/client`,
  see TODO there)
- Postgres schema for the register (biographic table + encrypted template
  table, kept separate on purpose)
- Liveness detection, this is a hardware/firmware concern, not something
  this service layer can meaningfully fake without real sensor SDK access
- Auth between go-client and java-matcher, fine on localhost during
  development, not fine before this goes anywhere near a real network
