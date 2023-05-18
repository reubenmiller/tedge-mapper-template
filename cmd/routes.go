/*
Copyright © 2023 thin-edge thinedge@thin-edge.io
*/
package cmd

import (
	"github.com/spf13/cobra"
)

// routesCmd represents the template command
var routesCmd = &cobra.Command{
	Use:   "routes",
	Short: "Routes command",
	Long:  `Routes are used to transform input data to new messages`,
	Run:   func(cmd *cobra.Command, args []string) {},
}

func init() {
	rootCmd.AddCommand(routesCmd)
}
