package app

import "github.com/spf13/cobra"

type OpenAPIExportCommand struct {
	Run func() error
}

func (c *OpenAPIExportCommand) Command() *cobra.Command {
	openapi := &cobra.Command{Use: "openapi", Short: "OpenAPI commands"}
	openapi.AddCommand(&cobra.Command{Use: "export", Short: "Export the OpenAPI document", RunE: func(_ *cobra.Command, _ []string) error {
		return c.Run()
	}})
	return openapi
}
