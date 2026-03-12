package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Generate devlb.yaml configuration template",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := getConfigPath()
		dir := getConfigDir()

		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}

		if _, err := os.Stat(path); err == nil {
			if isJSON() {
				return printJSON(map[string]any{
					"config_path":    path,
					"already_exists": true,
				})
			}
			return fmt.Errorf("config file already exists: %s", path)
		}

		template := `services:
  - name: api
    port: 3000
  - name: auth
    port: 8995
  # Add more services as needed
`
		if err := os.WriteFile(path, []byte(template), 0644); err != nil {
			return err
		}

		if isJSON() {
			return printJSON(map[string]any{
				"config_path": path,
			})
		}

		fmt.Printf("Config file created: %s\n", path)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
