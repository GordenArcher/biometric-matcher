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

### 2. Generate TLS certs

```
chmod +x scripts/gen-certs.sh
./scripts/gen-certs.sh
```

The matcher requires mutual TLS, no plaintext fallback, it will refuse
to start without these. See `certs/` output for what gets generated,
none of it is committed, regenerate any time.

### 3. Java matcher

No `gradlew` is committed in this scaffold (generating one needs a real
`gradle` install and network access to Gradle's distribution servers).
Two options:

**Docker (recommended, no local Gradle needed):**

```
docker-compose up --build
```

Builds against `gradle:8.9-jdk21` directly, see `java-matcher/Dockerfile`.
The compose file mounts `./certs` into the container and points
`MATCHER_TLS_CERT`/`MATCHER_TLS_KEY`/`MATCHER_TLS_CLIENT_CA` at the
right files, run step 2 first or this container won't start.

**Local, if you have Gradle installed** (`brew install gradle` on macOS):

```
cd java-matcher
MATCHER_TLS_CERT=../certs/server-cert.pem \
MATCHER_TLS_KEY=../certs/server-key.pem \
MATCHER_TLS_CLIENT_CA=../certs/ca-cert.pem \
MATCHER_TLS_REVOKED_SERIALS=../certs/revoked-serials.txt \
gradle run
```

The first time you do this, it's worth running `gradle wrapper` too and
committing the result (`gradlew`, `gradlew.bat`, `gradle/wrapper/`), so
nobody else on this repo needs Gradle installed locally either.

Either way, `MatcherServiceImpl.java` has real enroll/verify/identify
logic wired to SourceAFIS, not stubs, see the comments there for the
reasoning behind the match threshold and DPI constant.

### 4. Get some real fingerprint data to test with

```
chmod +x scripts/fetch-testdata.sh
./scripts/fetch-testdata.sh
```

The executable bit doesn't always survive a copy/paste or a zip extract,
so `chmod +x` first if you get a "permission denied".

Pulls FVC2002 DB1_B, the dataset SourceAFIS's own tutorial recommends for
trying it out. See `testdata/README.md` for match/non-match test pairs.

### 5. Go CLI

The CLI needs its own TLS material to talk to the matcher, either copy
`go-client/.env.example` to `go-client/.env` and fill in the four
`MATCHER_*` paths (already pointed at `../certs/...` by default, matching
step 2's output), or export them directly:

```
export MATCHER_CLIENT_CERT_FILE=../certs/client-cert.pem
export MATCHER_CLIENT_KEY_FILE=../certs/client-key.pem
export MATCHER_CA_FILE=../certs/ca-cert.pem
export MATCHER_REVOKED_SERIALS_FILE=../certs/revoked-serials.txt
```

```
cd go-client
go mod tidy
go run ./cmd/biometric-cli enroll --scan ../testdata/fvc2002-db1/101_1.tif --out ../testdata/template-101.bin
go run ./cmd/biometric-cli verify --scan ../testdata/fvc2002-db1/101_2.tif --template ../testdata/template-101.bin
```

`enroll`/`verify` above are for testing the matcher directly against raw
scan data, no database involved, templates just get written to local
files. `register`/`verify-person` below are the real path.

### 6. Real registration path (encryption + Postgres)

Postgres runs via the `postgres` service in `docker-compose.yml`
(`docker-compose up` brings both it and the matcher up together), schema
in `go-client/migrations/0001_init.sql` applies automatically on first
start via Postgres's `docker-entrypoint-initdb.d` convention.

Generate a real AES-256 key (32 random bytes, base64 encoded), then either
export it and `DATABASE_URL` directly, or copy `go-client/.env.example` to
`go-client/.env` and fill it in, the CLI loads `.env` automatically via
[godenv](https://github.com/GordenArcher/godenv):

```
cp go-client/.env.example go-client/.env
# edit go-client/.env, set TEMPLATE_ENCRYPTION_KEY=$(openssl rand -base64 32)
```

or without a `.env` file:

```
export TEMPLATE_ENCRYPTION_KEY=$(openssl rand -base64 32)
export DATABASE_URL="postgres://biometric:biometric_dev_only@localhost:5544/biometric?sslmode=disable"
```

Then:

```
cd go-client

go run ./cmd/biometric-cli register \
  --scan ../testdata/fvc2002-db1/101_1.tif \
  --name "Gorden Archer" \
  --dob 2000-01-01

go run ./cmd/biometric-cli verify-person \
  --scan ../testdata/fvc2002-db1/101_2.tif \
  --person <the person ID register printed>
```

`register` calls the matcher's `Enroll`, encrypts the resulting template,
and writes both the biographic row and the encrypted template in one
transaction. `verify-person` pulls the stored ciphertext, decrypts it in
Go, and only then sends plaintext to the matcher over gRPC, the matcher
never sees ciphertext or knows storage exists at all.

For a 1:N search against everyone registered so far:

```
go run ./cmd/biometric-cli identify-person --scan ../testdata/fvc2002-db1/101_2.tif
```

`identify-person` pages through `Store.ListTemplates` (Go's job per the
proto contract, the matcher never sees "the whole register"), decrypts
each row, and hands the matcher a plaintext candidate batch. `-limit`
and `-offset` control the page size if the register grows past a
comfortable single batch.

The encryption key itself comes from `internal/crypto.KeyProvider`, an
interface rather than a direct env var read. `EnvKeyProvider` is the only
implementation right now, swapping in a real KMS later is a new
implementation of that interface, not a rewrite of the encryption code
in `internal/crypto/template.go`.

## Verified working

Tested end to end against real FVC2002 DB1_B fingerprint scans, full
round trip through the Go CLI, gRPC, and the Java/SourceAFIS matcher:

```
$ go run ./cmd/biometric-cli enroll --scan ../testdata/fvc2002-db1/101_1.tif --out ../testdata/template-101.bin
enrolled, quality score 0.00, template written to ../testdata/template-101.bin

$ go run ./cmd/biometric-cli verify --scan ../testdata/fvc2002-db1/101_2.tif --template ../testdata/template-101.bin
match: true, score: 116.98

$ go run ./cmd/biometric-cli verify --scan ../testdata/fvc2002-db1/102_1.tif --template ../testdata/template-101.bin
match: false, score: 4.38
```

Same finger, different impression, scores 116.98, well past the 40.0
threshold. A genuinely different finger scores 4.38, nowhere close.
Quality score reads 0.00 as expected, see the note in
`MatcherServiceImpl.java` on why that field is intentionally unset.

## Cert rotation and revocation

`scripts/gen-certs.sh` produces one dev CA plus a server and client leaf
cert. Day-to-day, only the leaf certs need to change:

```
./scripts/rotate-certs.sh
```

Keeps the existing CA, issues fresh server/client certs signed by it, so
neither side needs to re-trust anything new. The old certs are backed up
to `certs/*.pem.old` before being overwritten, restart both the matcher
and any running CLI process to pick up the new certs.

If a cert may have been compromised, revoke it by serial rather than
waiting for it to expire:

```
./scripts/revoke-cert.sh certs/client-cert.pem.old
```

Both `RevocationCheckingTrustManager.java` (matcher side) and
`internal/client/revocation.go` (Go side) re-read
`certs/revoked-serials.txt` on every handshake, so a revocation takes
effect on the very next connection, no restart needed. This is a flat
file, not a real X.509 CRL or OCSP responder, deliberately, see the
comment in `scripts/revoke-cert.sh` for why that's the right amount of
machinery for two services sharing one CA rather than a real
distribution problem.

## Running the tests

Go, from `go-client`:

```
go test ./...
```

Covers `internal/crypto`'s `Encryptor` (round trip, wrong key, tampered
ciphertext, short input) and `EnvKeyProvider` (missing/invalid/valid env
var). No test touches Postgres or the matcher directly, those are
integration-shaped and covered by the "Verified working" run above
instead.

Java, from `java-matcher`:

```
gradle test
```

or via Docker if you don't have Gradle locally:

```
docker run --rm -v "$(pwd)":/app -w /app gradle:8.9-jdk21 gradle test --no-daemon
```

`MatcherServiceImplTest` runs real enroll/verify/identify calls against
FVC2002 DB1_B (same/different finger matching, and a 3-candidate
`identify` search), skipped automatically if `scripts/fetch-testdata.sh`
hasn't been run yet rather than failing.

## What's deliberately not built yet

- Liveness detection, this is a hardware/firmware concern, not something
  this service layer can meaningfully fake without real sensor SDK access
- A real KMS-backed `KeyProvider`, `EnvKeyProvider` is fine for local
  dev, the interface is designed so replacing it doesn't touch
  `internal/crypto/template.go`, but the replacement itself isn't written
