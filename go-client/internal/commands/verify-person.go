package commands

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/GordenArcher/godenv"

	"github.com/GordenArcher/biometric-matcher/internal/client"
	"github.com/GordenArcher/biometric-matcher/internal/crypto"
	"github.com/GordenArcher/biometric-matcher/internal/storage"
)

// RunVerifyPerson is the real verification path, distinct from RunVerify
// (which takes a plaintext template file for testing against raw scan
// data with no database involved). This is the flow that would back
// something like confirming identity before a card reprint, decrypt the
// stored template, hand plaintext to the matcher, never let the matcher
// see ciphertext.
func RunVerifyPerson(args []string) error {
	fs := flag.NewFlagSet("verify-person", flag.ExitOnError)
	scanPath := fs.String("scan", "", "path to a raw fingerprint scan file")
	personID := fs.String("person", "", "person ID to verify against")
	matcherAddr := fs.String("matcher", "localhost:50051", "matcher service address")
	dbDSN := fs.String("db", godenv.Get("DATABASE_URL", ""), "Postgres connection string, defaults to $DATABASE_URL (or .env)")
	keyEnv := fs.String("key-env", "TEMPLATE_ENCRYPTION_KEY", "env var holding the base64 AES-256 key")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *scanPath == "" || *personID == "" {
		return fmt.Errorf("-scan and -person are both required")
	}
	if *dbDSN == "" {
		return fmt.Errorf("-db or $DATABASE_URL must be set")
	}

	scanData, err := os.ReadFile(*scanPath)
	if err != nil {
		return fmt.Errorf("read scan file: %w", err)
	}

	ctx := context.Background()

	enc := crypto.NewEncryptor(crypto.EnvKeyProvider{EnvVar: *keyEnv})

	store, err := storage.Open(ctx, *dbDSN, enc)
	if err != nil {
		return err
	}
	defer store.Close()

	// Decryption happens here, before the matcher is ever contacted, the
	// matcher receives plaintext bytes over gRPC same as any other scan,
	// it has no idea this one came out of encrypted storage.
	storedTemplate, err := store.GetTemplate(ctx, *personID)
	if err != nil {
		return fmt.Errorf("load stored template: %w", err)
	}

	c, err := client.New(*matcherAddr)
	if err != nil {
		return err
	}
	defer c.Close()

	resp, err := c.Verify(ctx, scanData, storedTemplate)
	if err != nil {
		return err
	}

	fmt.Printf("match: %v, score: %.2f\n", resp.GetMatch(), resp.GetScore())
	return nil
}
