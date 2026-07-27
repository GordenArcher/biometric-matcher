#!/usr/bin/env bash
set -euo pipefail

# Rotates the server and client certs, keeps the existing CA. Anyone who
# already trusts ca-cert.pem doesn't need to do anything, the new leaf
# certs are automatically trusted, this is what makes rotation cheap
# compared to regenerating everything with scripts/gen-certs.sh, which
# would require redistributing a new CA to both sides.
CERT_DIR="$(dirname "$0")/../certs"
DAYS=90

if [[ ! -f "$CERT_DIR/ca-cert.pem" || ! -f "$CERT_DIR/ca-key.pem" ]]; then
  echo "No existing CA found in $CERT_DIR, run scripts/gen-certs.sh first." >&2
  exit 1
fi

cd "$CERT_DIR"

# Back up the old leaf certs before overwriting, so they can still be
# revoked by serial after rotation, once server-cert.pem is overwritten
# there'd be no way to look up the old serial to revoke it.
[[ -f server-cert.pem ]] && cp server-cert.pem server-cert.pem.old
[[ -f client-cert.pem ]] && cp client-cert.pem client-cert.pem.old

echo "Rotating server cert"
openssl genrsa -out server-key.pem 4096
openssl req -new -key server-key.pem -out server-csr.pem -subj "/CN=matcher"
openssl x509 -req -in server-csr.pem -CA ca-cert.pem -CAkey ca-key.pem \
  -CAcreateserial -out server-cert.pem -days $DAYS -sha256 \
  -extfile <(printf "subjectAltName=DNS:localhost,DNS:matcher,IP:127.0.0.1")

echo "Rotating client cert"
openssl genrsa -out client-key.pem 4096
openssl req -new -key client-key.pem -out client-csr.pem -subj "/CN=biometric-cli"
openssl x509 -req -in client-csr.pem -CA ca-cert.pem -CAkey ca-key.pem \
  -CAcreateserial -out client-cert.pem -days $DAYS -sha256

rm -f server-csr.pem client-csr.pem

echo
echo "New leaf certs issued (valid $DAYS days). The old certs still work"
echo "until they expire or are explicitly revoked, if the old ones may"
echo "have been compromised, revoke them now:"
echo "  ./scripts/revoke-cert.sh certs/server-cert.pem.old"
echo "  ./scripts/revoke-cert.sh certs/client-cert.pem.old"
echo "Restart both go-client and the matcher container to pick up the"
echo "new certs, an already-open connection keeps using whatever it"
echo "negotiated at dial time."