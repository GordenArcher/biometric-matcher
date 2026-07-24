package main

import (
	"fmt"
	"os"

	"github.com/GordenArcher/godenv"

	"github.com/GordenArcher/biometric-matcher/internal/commands"
)

// Manual subcommand routing instead of a CLI framework dependency, this
// tool only has five commands and isn't likely to grow flag complexity
// that would justify pulling in something like cobra.
func main() {
	// Ignored deliberately, not required is a valid state, register and
	// verify-person's -db/-key-env flags still fall back to real env vars
	// (export TEMPLATE_ENCRYPTION_KEY=... etc.) if no .env file exists,
	// this just makes a local .env file work too for convenience.
	_ = godenv.Load()

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "enroll":
		err = commands.RunEnroll(os.Args[2:])
	case "verify":
		err = commands.RunVerify(os.Args[2:])
	case "identify":
		err = commands.RunIdentify(os.Args[2:])
	case "register":
		err = commands.RunRegister(os.Args[2:])
	case "verify-person":
		err = commands.RunVerifyPerson(os.Args[2:])
	default:
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("usage: biometric-cli <enroll|verify|identify|register|verify-person> [flags]")
}
