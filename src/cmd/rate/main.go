package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/Darida/openrouterclient/src/api"
	"github.com/Darida/openrouterclient/src/model"
)

func main() {
	historyPath := flag.String("history", "", "history JSON file (required)")
	generationID := flag.String("id", "", "generation id to rate (required)")
	quality := flag.String("quality", "", "high, medium, or low (required)")
	reason := flag.String("reason", "", "why this rating (required)")
	flag.Parse()
	if *historyPath == "" || *generationID == "" || *quality == "" || *reason == "" {
		fail("--history, --id, --quality, and --reason are all required")
	}

	rater, err := api.NewRater(*historyPath)
	if err != nil {
		fail(err.Error())
	}
	if err := rater.Rate(context.Background(), *generationID, model.Quality(*quality), *reason); err != nil {
		fail(err.Error())
	}
	fmt.Fprintf(os.Stderr, "rate: recorded %s as %s\n", *generationID, *quality)
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, "rate:", message)
	os.Exit(1)
}
