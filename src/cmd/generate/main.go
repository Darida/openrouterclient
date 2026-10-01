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

	"github.com/Darida/openrouterclient/src/api"
	"github.com/Darida/openrouterclient/src/cmd/internal/requirementsfile"
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

	history, err := api.LocalHistory(*historyPath)
	if err != nil {
		fail(err.Error())
	}
	client, err := api.New(api.Config{
		APIKey:  apiKey,
		History: history,
		Replies: api.LocalReplies(),
		Logger:  logger,
	})
	if err != nil {
		fail(err.Error())
	}

	requirements, err := requirementsfile.Read(*inputPath, *tag, requirementsfile.Tier(*paid))
	if err != nil {
		fail(err.Error())
	}
	result, err := client.GenerateText(context.Background(), requirements)
	if err == nil {
		printJSON(os.Stdout, result)
	}
	// Stragglers from won races are still being recorded; exiting first would drop them.
	if closeErr := client.Close(); closeErr != nil {
		logger.Error("generate: recording stragglers failed", "err", closeErr)
		if err == nil {
			fail(closeErr.Error())
		}
	}
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

func rateLowCommand(rateScript, generationID, reason string) string {
	return strings.Join([]string{shellQuote(rateScript), shellQuote("--id=" + generationID), "--quality=low", shellQuote("--reason=" + reason)}, " ")
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
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
