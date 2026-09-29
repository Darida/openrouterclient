package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/Darida/openrouterclient/src/api"
	"github.com/Darida/openrouterclient/src/model"
)

func main() {
	inputPath := flag.String("input", "", "requirements JSON file (required)")
	historyPath := flag.String("history", "", "history JSON file (required)")
	tag := flag.String("tag", "", "history tag for this request (required)")
	rateScript := flag.String("rate-script", "", "path to bin/rate.sh, for the logged rating command (required)")
	paid := flag.Bool("paid", false, "use the cheapest paid models instead of free ones")
	flag.Parse()
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if *inputPath == "" || *historyPath == "" || *tag == "" || *rateScript == "" || apiKey == "" {
		fail("--input, --history, --tag, --rate-script, and OPENROUTER_API_KEY are all required")
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	client, err := api.New(api.Config{
		APIKey:      apiKey,
		HistoryPath: *historyPath,
		Logger:      logger,
	})
	if err != nil {
		fail(err.Error())
	}

	result, err := client.GenerateText(context.Background(), readRequirements(*inputPath, *tag, tierFor(*paid)))
	if err == nil {
		printJSON(os.Stdout, result)
	}
	// Stragglers from won races are still being recorded; exiting first would drop them.
	client.Close()
	if err != nil {
		var exhausted *model.AttemptsExhaustedError
		if errors.As(err, &exhausted) {
			for _, a := range exhausted.Attempts {
				if a.Outcome == model.OutcomeBelowTarget {
					logger.Info("generate: to reject this review and unrate its generation, run", "generation", a.GenerationID, "command", rateLowCommand(*rateScript, a.Review.GenerationID, "human rejected review"))
				}
			}
		}
		fail(err.Error())
	}
	if result.Review != nil {
		logger.Info("generate: to reject this review and unrate its generation, run", "command", rateLowCommand(*rateScript, result.Review.GenerationID, "human rejected review"))
	}
	logger.Info("generate: to rate this run as low quality, run", "command", rateLowCommand(*rateScript, result.GenerationID, "human rejected output"))
}

func readRequirements(path, tag string, tier model.ModelTier) model.TextGenerationRequirements {
	file, err := os.Open(path)
	if err != nil {
		fail(err.Error())
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var input requirementsFile
	if err := decoder.Decode(&input); err != nil {
		fail(fmt.Sprintf("%s: %v", path, err))
	}
	var maxCorrections int
	switch {
	case input.OutputValidationRules != "" && input.MaxCorrections == nil:
		fail(fmt.Sprintf("%s: maxCorrections is required when outputValidationRules is set", path))
	case input.OutputValidationRules == "" && input.MaxCorrections != nil:
		fail(fmt.Sprintf("%s: maxCorrections must be absent when outputValidationRules is empty", path))
	case input.MaxCorrections != nil:
		maxCorrections = *input.MaxCorrections
	}
	return model.TextGenerationRequirements{
		Prompt:                   input.Prompt,
		OutputSchema:             model.JSONSchema{Name: input.OutputSchema.Name, Schema: input.OutputSchema.Schema},
		OutputValidationRules:    input.OutputValidationRules,
		ReviewToleranceThreshold: input.ReviewToleranceThreshold,
		MaxCorrections:           maxCorrections,
		TargetQuality:            input.TargetQuality,
		Tag:                      tag,
		ModelTier:                tier,
		Timeout:                  time.Duration(input.TimeoutSeconds) * time.Second,
		MaxOutputTokens:          input.MaxOutputTokens,
		ExcludedModels:           input.ExcludedModels,
	}
}

func rateLowCommand(rateScript, generationID, reason string) string {
	return strings.Join([]string{shellQuote(rateScript), shellQuote("--id=" + generationID), "--quality=low", shellQuote("--reason=" + reason)}, " ")
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func tierFor(paid bool) model.ModelTier {
	if paid {
		return model.ModelTierPaid
	}
	return model.ModelTierFree
}

func printJSON(file *os.File, value any) {
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		fail(err.Error())
	}
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, "generate:", message)
	os.Exit(1)
}
