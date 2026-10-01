package main

import (
	"fmt"
	"os"

	"github.com/evanw/esbuild/pkg/api"
)

func main() {
	opts := api.BuildOptions{
		EntryPointsAdvanced: []api.EntryPoint{
			{InputPath: "public/js/src/app.js", OutputPath: "bundle"},
		},
		Outdir:            "public/js",
		Bundle:            true,
		Format:            api.FormatIIFE,
		MinifyWhitespace:  true,
		MinifyIdentifiers: true,
		MinifySyntax:      true,
		Write:             true,
		LogLevel:          api.LogLevelInfo,
	}

	if len(os.Args) > 1 && os.Args[1] == "--watch" {
		ctx, err := api.Context(opts)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if watchErr := ctx.Watch(api.WatchOptions{}); watchErr != nil {
			fmt.Fprintln(os.Stderr, watchErr)
			os.Exit(1)
		}
		fmt.Println("watching public/js/src/")
		select {}
	} else {
		result := api.Build(opts)
		if len(result.Errors) > 0 {
			os.Exit(1)
		}
	}
}
