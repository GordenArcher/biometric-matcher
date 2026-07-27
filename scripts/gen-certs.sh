#!/usr/bin/env bash
set -euo pipefail

# Self-signed CA used to sign both the server (matcher) and client
# (go-client) certs. One CA rather than two separate trust roots, this
# is two services we both control end to end, not a public-facing
# system, a single internal CA is the right amount of ceremony here.
CERT_DIR="$(dirname "$0")/../certs"
DAYS=825

mkdir -p "$CERT_DIR"
cd "$CERT_DIR"

echo "Generating CA"
openssl genrsa -out ca-key.pem 4096
openssl req -x509 -new -nodes -key ca-key.pem -sha256 -days $DAYS \
  -out ca-cert.pem -subj "/CN=biometric-matcher-dev-ca"

echo "Generating server (matcher) cert"
openssl genrsa -out server-key.pem 4096
openssl req -new -key server-key.pem -out server-csr.pem \
  -subj "/CN=matcher"
# SAN is required, most modern TLS clients (Go's crypto/tls included)
# ignore a bare CN and only check SubjectAltName, this must list
# whatever hostname the Go client dials, localhost/127.0.0.1 for a
# docker-compose port-mapped setup.
openssl x509 -req -in server-csr.pem -CA ca-cert.pem -CAkey ca-key.pem \
  -CAcreateserial -out server-cert.pem -days $DAYS -sha256 \
  -extfile <(printf "subjectAltName=DNS:localhost,DNS:matcher,IP:127.0.0.1")

echo "Generating client (go-client) cert"
openssl genrsa -out client-key.pem 4096
openssl req -new -key client-key.pem -out client-csr.pem \
  -subj "/CN=biometric-cli"
openssl x509 -req -in client-csr.pem -CA ca-cert.pem -CAkey ca-key.pem \
  -CAcreateserial -out client-cert.pem -days $DAYS -sha256

rm -f server-csr.pem client-csr.pem ca-cert.srl

# Empty, not missing, both sides treat a missing file as an error rather
# than "nothing is revoked", this makes that the explicit starting state
# instead of relying on scripts/revoke-cert.sh to create it later.
touch revoked-serials.txt

echo "Done. Certs written to $CERT_DIR:"
echo "  ca-cert.pem      - trust root for both sides"
echo "  server-*.pem     - matcher's cert/key, mount into the matcher container"
echo "  client-*.pem     - go-client's cert/key, used by the CLI"
echo "  revoked-serials.txt - empty for now, see scripts/revoke-cert.sh"
echo
echo "None of these are committed, see .gitignore. Regenerate any time"
echo "with this script, both sides just need to agree on the same CA."