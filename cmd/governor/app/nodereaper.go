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

	"github.com/keikoproj/governor/pkg/reaper/nodereaper"
	"github.com/spf13/cobra"
)

var nodeReaperArgs nodereaper.Args

// reapCmd represents the reap command
var nodeReapCmd = &cobra.Command{
	Use:   "node",
	Short: "node invokes the node reaper",
	Long:  `node reaper finds and force terminates instances of nodes which are not ready`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := nodereaper.Run(&nodeReaperArgs); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	},
}

func init() {
	reapCmd.AddCommand(nodeReapCmd)
	nodeReapCmd.Flags().StringVar(&nodeReaperArgs.K8sConfigPath, "kubeconfig", "", "Absolute path to the kubeconfig file")
	nodeReapCmd.Flags().StringVar(&nodeReaperArgs.KubectlLocalPath, "kubectl", "/usr/local/bin/kubectl", "Absolute path to the kubectl binary")
	nodeReapCmd.Flags().StringVar(&nodeReaperArgs.EC2Region, "region", "", "AWS Region to operate in")
	nodeReapCmd.Flags().Float64Var(&nodeReaperArgs.ReapAfter, "reap-after", 10, "Reaping threshold in minutes")
	nodeReapCmd.Flags().BoolVar(&nodeReaperArgs.LocalMode, "local-mode", false, "Use cluster external auth")
	nodeReapCmd.Flags().BoolVar(&nodeReaperArgs.DryRun, "dry-run", false, "Will not terminate node instances")
	nodeReapCmd.Flags().BoolVar(&nodeReaperArgs.ReapOld, "reap-old", false, "Terminate nodes older than --reap-old-threshold days")
	nodeReapCmd.Flags().BoolVar(&nodeReaperArgs.SoftReap, "soft-reap", true, "Will not terminate nodes with running pods")
	nodeReapCmd.Flags().BoolVar(&nodeReaperArgs.ReapUnknown, "reap-unknown", true, "Terminate nodes where State = Unknown")
	nodeReapCmd.Flags().BoolVar(&nodeReaperArgs.ReapUnready, "reap-unready", true, "Terminate nodes where State = NotReady")
	nodeReapCmd.Flags().BoolVar(&nodeReaperArgs.ReapGhost, "reap-ghost", true, "Prune nodes who's instances are already terminated")
	nodeReapCmd.Flags().BoolVar(&nodeReaperArgs.ReapUnjoined, "reap-unjoined", false, "Reap nodes which fail to join the cluster")
	nodeReapCmd.Flags().BoolVar(&nodeReaperArgs.ReapFlappy, "reap-flappy", true, "Terminate nodes which have flappy readiness")
	nodeReapCmd.Flags().BoolVar(&nodeReaperArgs.AsgValidation, "asg-validation", true, "Validate AutoScalingGroup's Min and Desired match before reaping")
	nodeReapCmd.Flags().Int32Var(&nodeReaperArgs.FlapCount, "flap-count", 5, "Only reap instances which have flapped atleast N times over the last hour")
	nodeReapCmd.Flags().Int64Var(&nodeReaperArgs.ReapThrottle, "reap-throttle", 300, "Post terminate wait in seconds for unhealthy nodes")
	nodeReapCmd.Flags().IntVar(&nodeReaperArgs.MaxKill, "max-kill-nodes", 1, "Kill up to N nodes per job run, considering throttle wait times")
	nodeReapCmd.Flags().Int64Var(&nodeReaperArgs.AgeReapThrottle, "reap-old-throttle", 1800, "Post terminate wait in seconds for old nodes")
	nodeReapCmd.Flags().Int32Var(&nodeReaperArgs.ReapOldThresholdMinutes, "reap-old-threshold-minutes", 43200, "Reap N minute old nodes")
	nodeReapCmd.Flags().Int32Var(&nodeReaperArgs.ReapUnjoinedThresholdMinutes, "reap-unjoined-threshold-minutes", 15, "Reap N minute old nodes")
	nodeReapCmd.Flags().StringVar(&nodeReaperArgs.ReapUnjoinedKey, "reap-unjoined-tag-key", "", "BE CAREFUL! EC2 tag key that identfies a joining node")
	nodeReapCmd.Flags().StringVar(&nodeReaperArgs.ReapUnjoinedValue, "reap-unjoined-tag-value", "", "BE CAREFUL! EC2 tag value that identfies a joining node")
	nodeReapCmd.Flags().StringArrayVar(&nodeReaperArgs.ReapTainted, "reap-tainted", []string{}, "marks nodes with a given taint reapable, must be in format of comma separated taints key=value:effect, key:effect or key")
	nodeReapCmd.Flags().Float64Var(&nodeReaperArgs.ReconsiderUnreapableAfter, "reconsider-unreapable-after", 10, "Time (in minutes) after which reconsider unreapable nodes")
}
