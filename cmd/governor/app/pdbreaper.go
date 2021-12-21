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

	"github.com/keikoproj/governor/pkg/reaper/pdbreaper"
	"github.com/spf13/cobra"
)

var pdbReaperArgs pdbreaper.Args

// pdbReaperCmd represents the reap command
var pdbReaperCmd = &cobra.Command{
	Use:   "pdb",
	Short: "pdb invokes the pdb reaper",
	Long:  `reap finds and force deletes pod disruption budgets which are stuck or misconfigured`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := pdbreaper.Run(&pdbReaperArgs); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	},
}

func init() {
	reapCmd.AddCommand(pdbReaperCmd)
	pdbReaperCmd.Flags().StringVar(&pdbReaperArgs.K8sConfigPath, "kubeconfig", "", "Absolute path to the kubeconfig file")
	pdbReaperCmd.Flags().BoolVar(&pdbReaperArgs.LocalMode, "local-mode", false, "Use cluster external auth")
	pdbReaperCmd.Flags().BoolVar(&pdbReaperArgs.DryRun, "dry-run", false, "Will not actually delete PDBs")
	pdbReaperCmd.Flags().BoolVar(&pdbReaperArgs.ReapMisconfigured, "reap-misconfigured", true, "Delete PDBs which are configured to not allow disruptions")
	pdbReaperCmd.Flags().BoolVar(&pdbReaperArgs.ReapMultiple, "reap-multiple", true, "Delete multiple PDBs which are targeting a single deployment")
	pdbReaperCmd.Flags().BoolVar(&pdbReaperArgs.ReapCrashLoop, "reap-crashloop", false, "Delete PDBs which are targeting a deployment whose pods are in a crashloop")
	pdbReaperCmd.Flags().BoolVar(&pdbReaperArgs.AllCrashLoop, "all-crashloop", true, "Only deletes PDBs for crashlooping pods when all pods are in crashloop")
	pdbReaperCmd.Flags().IntVar(&pdbReaperArgs.CrashLoopRestartCount, "crashloop-restart-count", 5, "Minimum restart count to when considering pods in crashloop")
	pdbReaperCmd.Flags().StringSliceVar(&pdbReaperArgs.ExcludedNamespaces, "excluded-namespaces", []string{}, "Namespaces excluded from scanning")
	pdbReaperCmd.Flags().BoolVar(&pdbReaperArgs.ReapNotReady, "reap-not-ready", true, "Deletes PDBs which have pods in not-ready state")
	pdbReaperCmd.Flags().IntVar(&pdbReaperArgs.ReapNotReadyThreshold, "not-ready-threshold-seconds", 1800, "Minimum seconds to wait when considering pods in not-ready state")
	pdbReaperCmd.Flags().BoolVar(&pdbReaperArgs.AllNotReady, "all-not-ready", false, "Only deletes PDBs for not-ready pods when all pods are in not-ready state")
	pdbReaperCmd.Flags().StringVar(&pdbReaperArgs.PromPushgateway, "prometheus-pushgateway", "", "Prometheus pushgateway URL")
}
