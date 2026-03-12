package cmd

import (
	"github.com/shouni/clibase"
	"github.com/spf13/cobra"
)

// Execute は、アプリケーションのメインエントリポイントです。
func Execute() {
	clibase.Execute(clibase.App{
		Name: "prototypus-ai-doc",
		Commands: []*cobra.Command{
			generateCmd,
		},
	})
}
