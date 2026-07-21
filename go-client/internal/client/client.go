package client

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/GordenArcher/biometric-matcher/gen/biometricpb"
)

// Client wraps the generated gRPC stub so CLI commands don't each have to
// know how to dial and set up the connection, and so the encryption step
// below has one obvious place to live once it's built.
type Client struct {
	conn *grpc.ClientConn
	rpc  biometricpb.MatcherClient
}

// Insecure transport credentials are fine for localhost dev against the
// java-matcher container, this must not ship as-is once this talks to
// anything over a real network, see README "what's not built yet".
func New(target string) (*Client, error) {
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial matcher at %s: %w", target, err)
	}

	return &Client{
		conn: conn,
		rpc:  biometricpb.NewMatcherClient(conn),
	}, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

// Enroll turns raw scan bytes into a template. The returned template is
// plaintext SourceAFIS output, encryption before persisting it is the
// caller's job (internal/commands), not this client's, keeps this layer
// a thin transport wrapper rather than mixing in storage concerns.
func (c *Client) Enroll(ctx context.Context, scanData []byte) (*biometricpb.EnrollResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
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
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
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
