package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/Darida/openrouterclient/src/api"
	"github.com/Darida/openrouterclient/src/model"
)

func main() {
	inputPath := flag.String("input", "", "requirements JSON file (required)")
	historyPath := flag.String("history", "", "history JSON file (required)")
	flag.Parse()
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if *inputPath == "" || *historyPath == "" || apiKey == "" {
		fail("--input, --history, and OPENROUTER_API_KEY are all required")
	}

	client, err := api.New(api.Config{
		APIKey:      apiKey,
		HistoryPath: *historyPath,
		Logger:      slog.New(slog.NewTextHandler(os.Stderr, nil)),
	})
	if err != nil {
		fail(err.Error())
	}

	result, err := client.GenerateText(context.Background(), readRequirements(*inputPath))
	var exhausted *model.AttemptsExhaustedError
	if errors.As(err, &exhausted) {
		printJSON(os.Stderr, exhausted)
	}
	if err == nil {
		printJSON(os.Stdout, result)
	}
	// Stragglers from won races are still being recorded; exiting first would drop them.
	client.Close()
	if err != nil {
		fail(err.Error())
	}
}

func readRequirements(path string) model.TextGenerationRequirements {
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
	return model.TextGenerationRequirements{
		Prompt:                input.Prompt,
		OutputSchema:          model.JSONSchema{Name: input.OutputSchema.Name, Schema: input.OutputSchema.Schema},
		OutputValidationRules: input.OutputValidationRules,
		TargetQuality:         input.TargetQuality,
	}
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
