/*
Copyright © 2023 Tessellated <tessellated.io>
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Binary name
const (
	binaryName = "restake-go"
	binaryIcon = "♻️"
)

// Version
var (
	RestakeVersion string
	GoVersion      string
	GitRevision    string
)

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Display the current version of Restake-Go",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("%s  %s:\n", binaryIcon, binaryName)
		fmt.Printf("  - Version: %s\n", RestakeVersion)
		fmt.Printf("  - Git Revision: %s\n", GitRevision)
		fmt.Printf("  - Go Version: %s\n", GoVersion)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
