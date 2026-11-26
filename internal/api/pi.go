package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"kamacu/internal/settings"
)

func preparePiCommand(ctx context.Context, args []string, command, discoveryCommand, model, modelArgument string) *sessionCreateError {
	if err := validatePiLaunch(args, model, modelArgument); err != nil {
		return &sessionCreateError{http.StatusBadRequest, err.Error()}
	}
	binary, err := exec.LookPath(args[0])
	if err != nil {
		return &sessionCreateError{http.StatusConflict, "Pi was not found on the Kamacu host; install it or update the agent command"}
	}
	binary, err = filepath.Abs(binary)
	if err != nil {
		return &sessionCreateError{http.StatusConflict, "could not resolve the Pi executable"}
	}
	args[0] = binary
	if ctx == nil {
		ctx = context.Background()
	}
	// The MCP bridge waits ten seconds for a start response. Keep the
	// combined version and model probes within one shorter deadline.
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if err := checkPiVersion(ctx, binary); err != nil {
		return &sessionCreateError{http.StatusConflict, err.Error()}
	}
	if model == "" {
		return nil
	}
	builtinDiscovery := strings.TrimSpace(discoveryCommand) == ""
	catalogCommand, err := modelDiscoveryArgs("pi", command, discoveryCommand)
	if err != nil {
		// A custom command with arguments can use a catalog that the generic
		// probe cannot represent. Pi resolves the saved model at runtime.
		if builtinDiscovery {
			return nil
		}
		return &sessionCreateError{http.StatusBadRequest, err.Error()}
	}
	if builtinDiscovery {
		catalogCommand[0] = binary
	}
	models, err := runModelDiscovery(ctx, catalogCommand, "pi", builtinDiscovery)
	if err != nil {
		return &sessionCreateError{http.StatusConflict, "could not verify the selected Pi model: " + err.Error()}
	}
	if !slices.Contains(models, model) {
		return &sessionCreateError{http.StatusConflict, "the selected Pi model is unavailable; choose another model or Agent default"}
	}
	return nil
}

func validatePiLaunch(args []string, model, modelArgument string) error {
	if len(args) == 0 {
		return errors.New("Pi agent command is empty")
	}
	for _, arg := range args[1:] {
		if arg == "--" {
			break
		}
		flag, _, _ := strings.Cut(arg, "=")
		switch flag {
		case "--session", "--session-id", "--session-dir", "--no-session", "--continue", "-c", "--resume", "-r", "--fork":
			return fmt.Errorf("Kamacu manages Pi sessions; remove %s from the agent command", flag)
		case "--mode", "--print", "-p", "--export", "--list-models", "--help", "-h", "--version", "-v":
			return fmt.Errorf("Kamacu runs Pi in interactive mode; remove %s from the agent command", flag)
		case "--provider", "--models":
			if model != "" {
				return fmt.Errorf("remove %s from the Pi agent command to use a task model override", flag)
			}
		}
	}
	if model == "" {
		return nil
	}
	if err := validatePiModel(model); err != nil {
		return err
	}
	if !slices.Equal(settings.Tokenize(modelArgument), []string{"--model", "{{model}}"}) {
		return errors.New("Pi requires the model argument --model {{model}}")
	}
	return nil
}

func validatePiModel(model string) error {
	model = strings.TrimSpace(model)
	provider, id, qualified := strings.Cut(model, "/")
	if !qualified || provider == "" || id == "" || strings.ContainsAny(model, " \t\r\n") {
		return errors.New("Pi model IDs must use provider/model-id")
	}
	return nil
}

func checkPiVersion(ctx context.Context, binary string) error {
	out, err := runCatalogCommand(ctx, []string{binary, "--version"}, "", 5*time.Second)
	if err != nil {
		return errors.New("could not read the Pi version; Kamacu requires Pi 1.6.1 or later")
	}
	var major, minor, patch int
	n, _ := fmt.Sscanf(strings.TrimPrefix(strings.TrimSpace(string(out)), "v"), "%d.%d.%d", &major, &minor, &patch)
	if n != 3 || major < 1 || major == 1 && (minor < 6 || minor == 6 && patch < 1) {
		return errors.New("Kamacu requires Pi 1.6.1 or later")
	}
	return nil
}
