package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/GordenArcher/godenv"

	"github.com/GordenArcher/biometric-matcher/gen/biometricpb"
)

// Client wraps the generated gRPC stub so CLI commands don't each have to
// know how to dial and set up the connection, and so the encryption step
// below has one obvious place to live once it's built.
type Client struct {
	conn *grpc.ClientConn
	rpc  biometricpb.MatcherClient
}

// New dials the matcher over mutual TLS. There is no insecure fallback,
// three env vars are required, missing any of them is a hard error
// rather than silently downgrading to plaintext, that downgrade is
// exactly the kind of thing that's fine in a demo and forgotten before
// this goes near a real network.
func New(target string) (*Client, error) {
	creds, err := loadTLSCredentials()
	if err != nil {
		return nil, fmt.Errorf("load tls credentials: %w", err)
	}

	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("dial matcher at %s: %w", target, err)
	}

	return &Client{
		conn: conn,
		rpc:  biometricpb.NewMatcherClient(conn),
	}, nil
}

// loadTLSCredentials builds a client-side TLS config presenting our own
// cert (so the matcher's mTLS clientAuth REQUIRE is satisfied) and
// trusting the shared dev CA (so we verify the matcher isn't an
// impostor). All paths come from env/.env via godenv, generated locally
// with scripts/gen-certs.sh, never committed.
func loadTLSCredentials() (credentials.TransportCredentials, error) {
	certFile := godenv.Get("MATCHER_CLIENT_CERT_FILE", "")
	keyFile := godenv.Get("MATCHER_CLIENT_KEY_FILE", "")
	caFile := godenv.Get("MATCHER_CA_FILE", "")
	revokedFile := godenv.Get("MATCHER_REVOKED_SERIALS_FILE", "")

	if certFile == "" || keyFile == "" || caFile == "" || revokedFile == "" {
		return nil, fmt.Errorf(
			"MATCHER_CLIENT_CERT_FILE, MATCHER_CLIENT_KEY_FILE, MATCHER_CA_FILE, and MATCHER_REVOKED_SERIALS_FILE must all be set, run scripts/gen-certs.sh if certs don't exist yet")
	}

	clientCert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load client cert/key: %w", err)
	}

	caPEM, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("read ca cert: %w", err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("ca cert at %s did not contain a valid PEM certificate", caFile)
	}

	return credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      caPool,
		// Must match a SAN entry on the server cert scripts/gen-certs.sh
		// generates (localhost, matcher, 127.0.0.1), not just be "the
		// hostname", TLS verification checks this literally.
		ServerName: "localhost",
		// go's tls package already ran normal chain verification against
		// RootCAs before calling this, InsecureSkipVerify defaults to
		// false, this callback only adds the extra revocation check on
		// top, it isn't standing in for certificate validation itself.
		VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			if len(verifiedChains) == 0 || len(verifiedChains[0]) == 0 {
				return fmt.Errorf("no verified chain to check for revocation")
			}

			revoked, err := loadRevokedSerials(revokedFile)
			if err != nil {
				// Fail closed, mirrors RevocationCheckingTrustManager on
				// the Java side, "can't check" must not mean "assume ok".
				return fmt.Errorf("check revocation: %w", err)
			}

			leaf := verifiedChains[0][0]
			if _, isRevoked := revoked[certSerialHex(leaf)]; isRevoked {
				return fmt.Errorf("matcher's certificate (serial %s) has been revoked", certSerialHex(leaf))
			}
			return nil
		},
	}), nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

// Enroll turns raw scan bytes into a template. The returned template is
// plaintext SourceAFIS output, encryption before persisting it is the
// caller's job (internal/commands), not this client's, keeps this layer
// a thin transport wrapper rather than mixing in storage concerns.
func (c *Client) Enroll(ctx context.Context, scanData []byte) (*biometricpb.EnrollResponse, error) {
	// 30s rather than a tighter number, the first call to a freshly
	// started matcher container pays JVM warmup and SourceAFIS class
	// loading cost that a warm container never does again, a 10s budget
	// here failed exactly that way in practice.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req := &biometricpb.EnrollRequest{
		ScanData: scanData,
		Format:   biometricpb.ScanFormat_SCAN_FORMAT_ISO_19794_2,
	}

	resp, err := c.rpc.Enroll(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("enroll rpc: %w", err)
	}
	return resp, nil
}

// Verify checks a fresh scan against one already-decrypted candidate
// template. Decryption of the stored template happens before this is
// called, the matcher never sees ciphertext.
func (c *Client) Verify(ctx context.Context, probeScan, candidateTemplate []byte) (*biometricpb.VerifyResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req := &biometricpb.VerifyRequest{
		ProbeScan:         probeScan,
		Format:            biometricpb.ScanFormat_SCAN_FORMAT_ISO_19794_2,
		CandidateTemplate: candidateTemplate,
	}

	resp, err := c.rpc.Verify(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("verify rpc: %w", err)
	}
	return resp, nil
}

// Identify runs a 1:N search. Timeout is longer than Enroll/Verify since
// a real candidate batch (even paginated) takes meaningfully longer than
// a single comparison, a 10s timeout here would just cause spurious
// failures once there's real data volume.
func (c *Client) Identify(ctx context.Context, probeScan []byte, candidates []*biometricpb.TemplateCandidate) (*biometricpb.IdentifyResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	req := &biometricpb.IdentifyRequest{
		ProbeScan:  probeScan,
		Format:     biometricpb.ScanFormat_SCAN_FORMAT_ISO_19794_2,
		Candidates: candidates,
	}

	resp, err := c.rpc.Identify(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("identify rpc: %w", err)
	}
	return resp, nil
}
