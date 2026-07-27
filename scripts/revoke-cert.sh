#!/usr/bin/env bash
set -euo pipefail

# Appends a cert's serial number to certs/revoked-serials.txt. This is a
# deliberately simple revoked-list mechanism, not a spec-compliant X.509
# CRL or OCSP responder, no signed revocation object, no distribution
# point URL, just a flat file both sides read on every handshake. That's
# the right amount of machinery for two services sharing one dev CA,
# a real CRL/OCSP setup would be solving a distribution problem this
# project doesn't have.
if [[ $# -ne 1 ]]; then
  echo "usage: $0 <path-to-cert.pem>" >&2
  exit 1
fi

CERT_FILE="$1"
CERT_DIR="$(dirname "$0")/../certs"
REVOKED_FILE="$CERT_DIR/revoked-serials.txt"

if [[ ! -f "$CERT_FILE" ]]; then
  echo "no such file: $CERT_FILE" >&2
  exit 1
fi

SERIAL=$(openssl x509 -in "$CERT_FILE" -noout -serial | cut -d= -f2 | tr '[:lower:]' '[:upper:]')

if [[ -z "$SERIAL" ]]; then
  echo "could not read a serial number from $CERT_FILE" >&2
  exit 1
fi

touch "$REVOKED_FILE"
if grep -qx "$SERIAL" "$REVOKED_FILE"; then
  echo "serial $SERIAL is already revoked"
  exit 0
fi

echo "$SERIAL" >> "$REVOKED_FILE"
echo "revoked serial $SERIAL (from $CERT_FILE)"
echo "restart the matcher and any running CLI process for this to take effect immediately, both sides re-read the revoked list on every new connection anyway, so a fresh command already picks it up"