package main

import (
	"context"
	"encoding/json"
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
	tag := flag.String("tag", "", "history tag for this request (required)")
	paid := flag.Bool("paid", false, "use the cheapest paid models instead of free ones")
	flag.Parse()
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if *inputPath == "" || *historyPath == "" || *tag == "" || apiKey == "" {
		fail("--input, --history, --tag, and OPENROUTER_API_KEY are all required")
	}

	client, err := api.New(api.Config{
		APIKey:      apiKey,
		HistoryPath: *historyPath,
		Logger:      slog.New(slog.NewTextHandler(os.Stderr, nil)),
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
		fail(err.Error())
	}
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
	return model.TextGenerationRequirements{
		Prompt:                input.Prompt,
		OutputSchema:          model.JSONSchema{Name: input.OutputSchema.Name, Schema: input.OutputSchema.Schema},
		OutputValidationRules: input.OutputValidationRules,
		TargetQuality:         input.TargetQuality,
		Tag:                   tag,
		ModelTier:             tier,
		MaxOutputTokens:       input.MaxOutputTokens,
	}
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
