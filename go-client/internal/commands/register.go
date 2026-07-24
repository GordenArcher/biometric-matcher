package commands

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/GordenArcher/godenv"

	"github.com/GordenArcher/biometric-matcher/internal/client"
	"github.com/GordenArcher/biometric-matcher/internal/crypto"
	"github.com/GordenArcher/biometric-matcher/internal/storage"
)

// RunRegister is the real enrollment path, distinct from RunEnroll (which
// only exercises the matcher and writes a plaintext template file for
// local testing against raw fingerprint data). This command is what
// enroll should have looked like once encryption and Postgres exist,
// the matcher never sees this distinction, it's entirely a Go-side
// architectural choice.
func RunRegister(args []string) error {
	fs := flag.NewFlagSet("register", flag.ExitOnError)
	scanPath := fs.String("scan", "", "path to a raw fingerprint scan file")
	fullName := fs.String("name", "", "full name of the person being registered")
	dobRaw := fs.String("dob", "", "date of birth, YYYY-MM-DD")
	matcherAddr := fs.String("matcher", "localhost:50051", "matcher service address")
	dbDSN := fs.String("db", godenv.Get("DATABASE_URL", ""), "Postgres connection string, defaults to $DATABASE_URL (or .env)")
	keyEnv := fs.String("key-env", "TEMPLATE_ENCRYPTION_KEY", "env var holding the base64 AES-256 key")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *scanPath == "" || *fullName == "" || *dobRaw == "" {
		return fmt.Errorf("-scan, -name, and -dob are all required")
	}
	if *dbDSN == "" {
		return fmt.Errorf("-db or $DATABASE_URL must be set")
	}

	dob, err := time.Parse("2006-01-02", *dobRaw)
	if err != nil {
		return fmt.Errorf("parse -dob (want YYYY-MM-DD): %w", err)
	}

	scanData, err := os.ReadFile(*scanPath)
	if err != nil {
		return fmt.Errorf("read scan file: %w", err)
	}

	ctx := context.Background()

	c, err := client.New(*matcherAddr)
	if err != nil {
		return err
	}
	defer c.Close()

	enrollResp, err := c.Enroll(ctx, scanData)
	if err != nil {
		return err
	}

	// KeyProvider is EnvKeyProvider today only, swapping this one line
	// for a KMS-backed implementation is the entire migration path, per
	// the interface boundary in internal/crypto/keyprovider.go.
	enc := crypto.NewEncryptor(crypto.EnvKeyProvider{EnvVar: *keyEnv})

	store, err := storage.Open(ctx, *dbDSN, enc)
	if err != nil {
		return err
	}
	defer store.Close()

	personID, err := store.EnrollPerson(ctx, *fullName, dob, enrollResp.GetTemplate())
	if err != nil {
		return fmt.Errorf("store enrollment: %w", err)
	}

	fmt.Printf("registered %s as person %s\n", *fullName, personID)
	return nil
}
