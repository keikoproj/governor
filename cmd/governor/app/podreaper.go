/*

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package app

import (
	"fmt"
	"os"

	"github.com/keikoproj/governor/pkg/reaper/podreaper"
	"github.com/spf13/cobra"
)

var podReaperArgs podreaper.Args

// podReapCmd represents the reap command
var podReapCmd = &cobra.Command{
	Use:   "pod",
	Short: "pod invokes the pod reaper",
	Long:  `reap finds and force deletes pods stuck in Terminating`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx := &podreaper.ReaperContext{}
		if err := ctx.ValidateArguments(&podReaperArgs); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		if err := podreaper.Run(ctx); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	},
}

func init() {
	reapCmd.AddCommand(podReapCmd)
	podReapCmd.Flags().StringVar(&podReaperArgs.K8sConfigPath, "kubeconfig", "", "Absolute path to the kubeconfig file")
	podReapCmd.Flags().Float64Var(&podReaperArgs.ReapAfter, "reap-after", 10, "Reaping threshold in minutes")
	podReapCmd.Flags().BoolVar(&podReaperArgs.LocalMode, "local-mode", false, "Use cluster external auth")
	podReapCmd.Flags().BoolVar(&podReaperArgs.DryRun, "dry-run", false, "Will not terminate pods")
	podReapCmd.Flags().BoolVar(&podReaperArgs.SoftReap, "soft-reap", true, "Will not terminate pods with running containers")
	podReapCmd.Flags().BoolVar(&podReaperArgs.SoftReap, "reap-completed", false, "Delete pods in completed phase")
	podReapCmd.Flags().Float64Var(&podReaperArgs.ReapCompletedAfter, "reap-completed-after", 0, "Reaping threshold in minutes for completed pods")
	podReapCmd.Flags().BoolVar(&podReaperArgs.SoftReap, "reap-failed", false, "Delete pods in failed phase")
	podReapCmd.Flags().Float64Var(&podReaperArgs.ReapFailedAfter, "reap-failed-after", 0, "Reaping threshold in minutes for failed pods")
}
