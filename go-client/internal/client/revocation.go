package client

import (
	"crypto/x509"
	"fmt"
	"os"
	"strings"
)

// loadRevokedSerials reads the same flat file scripts/revoke-cert.sh
// writes to and RevocationCheckingTrustManager.java reads on the matcher
// side. One shared file, one shared format (uppercase hex, no leading
// zero stripped or added, matching openssl's `-noout -serial` output),
// deliberately not a real CRL, see scripts/revoke-cert.sh for why.
func loadRevokedSerials(path string) (map[string]struct{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		// Missing file is a real error here too, mirrors the Java side,
		// a missing file is not the same thing as "nothing is revoked".
		return nil, fmt.Errorf("read revoked serials file: %w", err)
	}

	revoked := make(map[string]struct{})
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.ToUpper(strings.TrimSpace(line))
		if trimmed != "" {
			revoked[trimmed] = struct{}{}
		}
	}
	return revoked, nil
}

// certSerialHex matches the format both scripts/revoke-cert.sh (via
// openssl -noout -serial) and the Java trust manager (via
// BigInteger.toString(16).toUpperCase()) produce, uppercase hex, no
// leading zero padding. big.Int.Text(16) already omits leading zeros,
// only the case needs correcting.
func certSerialHex(cert *x509.Certificate) string {
	return strings.ToUpper(cert.SerialNumber.Text(16))
}
