package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/Darida/openrouterclient/src/cmd/internal/requirementsfile"
	"github.com/Darida/openrouterclient/src/internal/engine"
	"github.com/Darida/openrouterclient/src/internal/history"
	"github.com/Darida/openrouterclient/src/internal/replyfile"
)

func main() {
	inputPath := flag.String("input", "", "requirements JSON file (required)")
	historyPath := flag.String("history", "", "history JSON file (required)")
	tag := flag.String("tag", "", "history tag whose exclusions apply (required)")
	paid := flag.Bool("paid", false, "list the cheapest paid models instead of free ones")
	flag.Parse()
	if *inputPath == "" || *historyPath == "" || *tag == "" {
		fail("--input, --history, and --tag are all required")
	}

	request, err := requirementsfile.Read(*inputPath, requirementsfile.Tier(*paid))
	if err != nil {
		fail(err.Error())
	}
	store, err := history.Open(*historyPath)
	if err != nil {
		fail(err.Error())
	}
	// Listing reads only the public model catalog, so no API key is sent.
	e := engine.New(engine.Production, "", *tag, requirementsfile.Timeout, store, replyfile.Local(), slog.New(slog.NewTextHandler(os.Stderr, nil)))
	candidates, err := e.CandidateModels(request)
	if err != nil {
		fail(err.Error())
	}
	for _, id := range candidates {
		fmt.Println(id)
	}
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, "models:", message)
	os.Exit(1)
}
