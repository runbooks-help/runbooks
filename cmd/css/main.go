// SPDX-License-Identifier: FSL-1.1-MIT

package main

import (
	"fmt"
	"os"

	"github.com/evanw/esbuild/pkg/api"
)

func main() {
	opts := api.BuildOptions{
		EntryPoints:      []string{"public/css/src/main.css"},
		Bundle:           true,
		MinifyWhitespace: true,
		MinifySyntax:     true,
		Outfile:          "public/css/bundle.css",
		Write:            true,
		LogLevel:         api.LogLevelInfo,
		External:         []string{"*.woff2"},
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
		fmt.Println("watching public/css/src/")
		select {}
	} else {
		result := api.Build(opts)
		if len(result.Errors) > 0 {
			os.Exit(1)
		}
	}
}
