package cli

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/huh"

	"github.com/kaminocorp/lumber/internal/config"
	"github.com/kaminocorp/lumber/internal/download"
)

// Sentinel errors for wizard outcomes.
var (
	ErrWizardCancelled    = errors.New("wizard cancelled by user")
	ErrModelDownloadDeclined = errors.New("model download declined")
)

// RunWizard displays an interactive setup wizard and returns a populated Config.
// It should only be called when stdin is a TTY and no connector is configured.
func RunWizard(base config.Config) (config.Config, error) {
	cfg := base

	printHeader(config.Version)

	// --- Form 1: Model Readiness ---
	if !ModelsReady(cfg) {
		if err := promptModelDownload(&cfg); err != nil {
			return cfg, err
		}
	}

	// --- Form 2: Source Selection ---
	var source string
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("How do you want to ingest logs?").
				Options(
					huh.NewOption("Local logs (file or pipe)", "local"),
					huh.NewOption("Cloud provider (live)", "cloud"),
				).
				Value(&source),
		),
	).Run()
	if err != nil {
		return cfg, wrapFormError(err)
	}

	if source == "local" {
		if err := promptLocalSource(&cfg); err != nil {
			return cfg, err
		}
	} else {
		if err := promptCloudSource(&cfg); err != nil {
			return cfg, err
		}
	}

	// --- Form 3: Output Options ---
	if err := promptOutputOptions(&cfg, source); err != nil {
		return cfg, err
	}

	// --- Form 4: Summary Confirmation ---
	if err := promptSummary(&cfg); err != nil {
		return cfg, err
	}

	// TTY sessions default to pretty output.
	cfg.Output.Pretty = true

	printReady(cfg.Connector.Provider, cfg.Mode)

	return cfg, nil
}

// ModelsReady checks whether model files and the ORT library are available.
func ModelsReady(cfg config.Config) bool {
	for _, path := range []string{cfg.Engine.ModelPath, cfg.Engine.VocabPath, cfg.Engine.ProjectionPath} {
		if _, err := os.Stat(path); err != nil {
			return false
		}
	}
	// Check ORT library in the model directory.
	_, libName, err := download.OrtPlatform()
	if err != nil {
		return false
	}
	ortDir := filepath.Dir(cfg.Engine.ModelPath)
	if _, err := os.Stat(filepath.Join(ortDir, libName)); err == nil {
		return true
	}
	// Check cache dir as fallback.
	cacheDir, err := download.DefaultCacheDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(cacheDir, libName))
	return err == nil
}

// promptModelDownload asks the user to download models or exit.
func promptModelDownload(cfg *config.Config) error {
	var shouldDownload bool
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Model files not found").
				Description("Lumber needs embedding model files (~50MB) and the ONNX Runtime library (~15MB) to classify logs. Download them now?").
				Affirmative("Yes, download").
				Negative("No, exit").
				Value(&shouldDownload),
		),
	).Run()
	if err != nil {
		return wrapFormError(err)
	}

	if !shouldDownload {
		fmt.Fprintf(os.Stderr, "\n  Model files are required. To download manually:\n")
		fmt.Fprintf(os.Stderr, "    make download-model    (from source checkout)\n")
		fmt.Fprintf(os.Stderr, "    See: https://github.com/kaminocorp/lumber#install\n\n")
		return ErrModelDownloadDeclined
	}

	cacheDir, err := download.DefaultCacheDir()
	if err != nil {
		return fmt.Errorf("resolving cache directory: %w", err)
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return fmt.Errorf("creating cache dir: %w", err)
	}

	fmt.Fprintf(os.Stderr, "  Downloading model files to %s ...\n", cacheDir)
	if err := download.DownloadModels(cacheDir); err != nil {
		return fmt.Errorf("model download failed: %w", err)
	}
	fmt.Fprintf(os.Stderr, "  Downloading ONNX Runtime ...\n")
	if err := download.DownloadORT(cacheDir); err != nil {
		return fmt.Errorf("ORT download failed: %w", err)
	}

	// Update config to point at cached models, deriving paths from download.ModelFiles.
	for _, mf := range download.ModelFiles {
		full := filepath.Join(cacheDir, mf.RelPath)
		switch mf.RelPath {
		case "model_quantized.onnx":
			cfg.Engine.ModelPath = full
		case "vocab.txt":
			cfg.Engine.VocabPath = full
		case filepath.Join("2_Dense", "model.safetensors"):
			cfg.Engine.ProjectionPath = full
		}
	}

	fmt.Fprintf(os.Stderr, "  %s\n\n", render(successStyle, "✓ Models ready"))
	return nil
}

// promptLocalSource asks the user to choose between file and stdin.
func promptLocalSource(cfg *config.Config) error {
	var localSource string
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Log source:").
				Options(
					huh.NewOption("Read a file", "file"),
					huh.NewOption("Pipe from stdin  (e.g. cat app.log | lumber)", "stdin"),
				).
				Value(&localSource),
		),
	).Run()
	if err != nil {
		return wrapFormError(err)
	}

	if localSource == "file" {
		var filePath string
		err := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("File path:").
					Placeholder("/var/log/app.log").
					Validate(func(s string) error {
						if strings.TrimSpace(s) == "" {
							return fmt.Errorf("file path is required")
						}
						if _, err := os.Stat(s); err != nil {
							return fmt.Errorf("file not accessible: %s", s)
						}
						return nil
					}).
					Value(&filePath),
			),
		).Run()
		if err != nil {
			return wrapFormError(err)
		}

		cfg.Connector.Provider = "file"
		if cfg.Connector.Extra == nil {
			cfg.Connector.Extra = make(map[string]string)
		}
		cfg.Connector.Extra["file"] = filePath
	} else {
		cfg.Connector.Provider = "stdin"
		fmt.Fprintf(os.Stderr, "  %s\n\n", render(mutedStyle, "Waiting for piped input... (usage: cat app.log | lumber)"))
	}

	// Local sources always stream.
	cfg.Mode = "stream"

	return nil
}

// promptCloudSource asks the user for provider, API key, and provider-specific config.
func promptCloudSource(cfg *config.Config) error {
	var provider string
	var apiKey string

	err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Provider:").
				Options(
					huh.NewOption("Vercel", "vercel"),
					huh.NewOption("Fly.io", "flyio"),
					huh.NewOption("Supabase", "supabase"),
				).
				Value(&provider),
			huh.NewInput().
				Title("API key:").
				EchoMode(huh.EchoModePassword).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("API key is required")
					}
					return nil
				}).
				Value(&apiKey),
		),
	).Run()
	if err != nil {
		return wrapFormError(err)
	}

	cfg.Connector.Provider = provider
	cfg.Connector.APIKey = apiKey
	if cfg.Connector.Extra == nil {
		cfg.Connector.Extra = make(map[string]string)
	}

	// Provider-specific prompts.
	switch provider {
	case "vercel":
		if err := promptVercelExtras(cfg); err != nil {
			return err
		}
	case "flyio":
		if err := promptFlyioExtras(cfg); err != nil {
			return err
		}
	case "supabase":
		if err := promptSupabaseExtras(cfg); err != nil {
			return err
		}
	}

	return nil
}

func promptVercelExtras(cfg *config.Config) error {
	var projectID, teamID string
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Project ID (optional):").Placeholder("prj_xxxxx").Value(&projectID),
			huh.NewInput().Title("Team ID (optional):").Placeholder("team_xxxxx").Value(&teamID),
		),
	).Run()
	if err != nil {
		return wrapFormError(err)
	}
	if projectID != "" {
		cfg.Connector.Extra["project_id"] = projectID
	}
	if teamID != "" {
		cfg.Connector.Extra["team_id"] = teamID
	}
	return nil
}

func promptFlyioExtras(cfg *config.Config) error {
	var appName string
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("App name:").
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("app name is required")
					}
					return nil
				}).
				Value(&appName),
		),
	).Run()
	if err != nil {
		return wrapFormError(err)
	}
	cfg.Connector.Extra["app_name"] = appName
	return nil
}

func promptSupabaseExtras(cfg *config.Config) error {
	var projectRef string
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Project ref:").
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("project ref is required")
					}
					return nil
				}).
				Value(&projectRef),
		),
	).Run()
	if err != nil {
		return wrapFormError(err)
	}
	cfg.Connector.Extra["project_ref"] = projectRef
	return nil
}

// promptOutputOptions asks about output destinations, verbosity, and mode.
func promptOutputOptions(cfg *config.Config, source string) error {
	var extraOutputs []string
	var verbosity string

	fields := []huh.Field{
		huh.NewMultiSelect[string]().
			Title("Output destinations:").
			Description("Stdout is always enabled. Select additional outputs:").
			Options(
				huh.NewOption("File (NDJSON)", "file"),
				huh.NewOption("Webhook (HTTP POST)", "webhook"),
			).
			Value(&extraOutputs),
		huh.NewSelect[string]().
			Title("Output verbosity:").
			Options(
				huh.NewOption("Standard (balanced)", "standard"),
				huh.NewOption("Minimal (most compact)", "minimal"),
				huh.NewOption("Full (everything)", "full"),
			).
			Value(&verbosity),
	}

	err := huh.NewForm(huh.NewGroup(fields...)).Run()
	if err != nil {
		return wrapFormError(err)
	}

	cfg.Engine.Verbosity = verbosity

	// File output path.
	if slices.Contains(extraOutputs, "file") {
		var outputFilePath string
		err := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Output file path:").
					Placeholder("lumber-output.ndjson").
					Value(&outputFilePath),
			),
		).Run()
		if err != nil {
			return wrapFormError(err)
		}
		if outputFilePath == "" {
			outputFilePath = "lumber-output.ndjson"
		}
		cfg.Output.FilePath = outputFilePath
	}

	// Webhook URL.
	if slices.Contains(extraOutputs, "webhook") {
		var webhookURL string
		err := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Webhook URL:").
					Placeholder("https://example.com/logs").
					Validate(func(s string) error {
						u, err := url.ParseRequestURI(s)
						if err != nil {
							return fmt.Errorf("invalid URL: %w", err)
						}
						if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
							return fmt.Errorf("invalid URL: must be a valid http:// or https:// URL with a host")
						}
						return nil
					}).
					Value(&webhookURL),
			),
		).Run()
		if err != nil {
			return wrapFormError(err)
		}
		cfg.Output.WebhookURL = webhookURL
	}

	// Mode selection (cloud only — local defaults to stream).
	if source == "cloud" {
		var mode string
		err := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Mode:").
					Options(
						huh.NewOption("Stream (live tail)", "stream"),
						huh.NewOption("Query (historical)", "query"),
					).
					Value(&mode),
			),
		).Run()
		if err != nil {
			return wrapFormError(err)
		}
		cfg.Mode = mode

		if mode == "query" {
			if err := promptQueryRange(cfg); err != nil {
				return err
			}
		}
	}

	return nil
}

func promptQueryRange(cfg *config.Config) error {
	var from, to string
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("From (RFC3339):").
				Placeholder("2026-01-01T00:00:00Z").
				Validate(func(s string) error {
					if _, err := time.Parse(time.RFC3339, s); err != nil {
						return fmt.Errorf("invalid RFC3339 time: %s", s)
					}
					return nil
				}).
				Value(&from),
			huh.NewInput().
				Title("To (RFC3339):").
				Placeholder("2026-01-01T01:00:00Z").
				Validate(func(s string) error {
					if _, err := time.Parse(time.RFC3339, s); err != nil {
						return fmt.Errorf("invalid RFC3339 time: %s", s)
					}
					return nil
				}).
				Value(&to),
		),
	).Run()
	if err != nil {
		return wrapFormError(err)
	}
	var parseErr error
	cfg.QueryFrom, parseErr = time.Parse(time.RFC3339, from)
	if parseErr != nil {
		return fmt.Errorf("parsing -from time: %w", parseErr)
	}
	cfg.QueryTo, parseErr = time.Parse(time.RFC3339, to)
	if parseErr != nil {
		return fmt.Errorf("parsing -to time: %w", parseErr)
	}
	if !cfg.QueryFrom.Before(cfg.QueryTo) {
		return fmt.Errorf("-from (%s) must be before -to (%s)", cfg.QueryFrom.Format(time.RFC3339), cfg.QueryTo.Format(time.RFC3339))
	}
	return nil
}

// promptSummary shows a summary of the configuration and asks for confirmation.
func promptSummary(cfg *config.Config) error {
	summary := buildSummary(cfg)

	var confirmed bool
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Ready to start").
				Description(summary).
				Affirmative("Start").
				Negative("Cancel").
				Value(&confirmed),
		),
	).Run()
	if err != nil {
		return wrapFormError(err)
	}
	if !confirmed {
		return ErrWizardCancelled
	}
	return nil
}

// wrapFormError distinguishes user-initiated cancellation (Ctrl+C / Esc)
// from unexpected form errors, wrapping the former with ErrWizardCancelled.
func wrapFormError(err error) error {
	if errors.Is(err, huh.ErrUserAborted) {
		return fmt.Errorf("%w: %w", ErrWizardCancelled, err)
	}
	return fmt.Errorf("wizard error: %w", err)
}

func buildSummary(cfg *config.Config) string {
	var b strings.Builder
	fmt.Fprintf(&b, "  Source:     %s\n", cfg.Connector.Provider)
	fmt.Fprintf(&b, "  Mode:       %s\n", cfg.Mode)
	fmt.Fprintf(&b, "  Verbosity:  %s\n", cfg.Engine.Verbosity)

	out := "stdout"
	if cfg.Output.FilePath != "" {
		out += " + file (" + cfg.Output.FilePath + ")"
	}
	if cfg.Output.WebhookURL != "" {
		out += " + webhook"
	}
	fmt.Fprintf(&b, "  Output:     %s", out)
	return b.String()
}
