// Command fetch-model-data downloads the LLM gateway's model prices and capabilities into
// pkg/modeldata/data, for a build to embed. It downloads them from the -url flag, or else
// MODEL_DATA_URL, and does nothing when neither is set: CI sets it for the images and release
// binaries it publishes. When it is set and the download fails or doesn't pass validation, it
// fails, so a published build never quietly ships without the data.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/obot-platform/obot/pkg/modeldata"
)

func main() {
	url := flag.String("url", os.Getenv("MODEL_DATA_URL"), "where to download the model data from, such as https://upgrade-server.obot.ai/v1/model-data (default $MODEL_DATA_URL)")
	out := flag.String("out", "pkg/modeldata/data", "the directory to write it to")
	flag.Parse()

	if *url == "" {
		fmt.Println("MODEL_DATA_URL isn't set; not bundling model data")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := modeldata.Bundle(ctx, *url, *out); err != nil {
		fmt.Fprintf(os.Stderr, "failed to download the model data from %s: %v\n", *url, err)
		os.Exit(1)
	}
	fmt.Printf("downloaded the model data from %s into %s\n", *url, *out)
}
