package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func main() {
	binaryName := "whatsrook"
	if runtime.GOOS == "windows" {
		binaryName = "whatsrook.exe"
	}

	execPath := filepath.Join(".", binaryName)

	// Ensure the binary is built if missing or if executed directly via `go run .`
	if _, err := os.Stat(execPath); os.IsNotExist(err) {
		fmt.Println("Binary 'whatsrook' not found. Building binary...")
		buildCmd := exec.Command("go", "build", "-o", binaryName, ".")
		buildCmd.Stdout = os.Stdout
		buildCmd.Stderr = os.Stderr
		if err := buildCmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to build whatsrook binary: %v\n", err)
			os.Exit(1)
		}
	}

	// Check if entrypoint.sh exists and is executable
	entrypointScript := "./entrypoint.sh"
	if _, err := os.Stat(entrypointScript); err == nil && runtime.GOOS != "windows" {
		cmd := exec.Command("/bin/sh", entrypointScript)
		cmd.Args = append(cmd.Args, os.Args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		cmd.Env = os.Environ()

		if err := cmd.Run(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				os.Exit(exitErr.ExitCode())
			}
			fmt.Fprintf(os.Stderr, "Failed to execute entrypoint script: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Fallback to running daemon logic directly if entrypoint script is absent or on Windows
	runDaemon()
}
