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

	"github.com/keikoproj/governor/pkg/reaper/cordon/aznat"
	"github.com/spf13/cobra"
)

var azNatArgs aznat.Args

// podReapCmd represents the reap command
var azNatCmd = &cobra.Command{
	Use:   "az-nat",
	Short: "az-nat cordons a NAT in a specific AZ",
	Long:  `az-nat modifies route tables to use NATs in different AZs`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := aznat.Run(&azNatArgs); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	},
}

func init() {
	cordonCmd.AddCommand(azNatCmd)
	azNatCmd.Flags().StringSliceVar(&azNatArgs.TargetAvailabilityZoneIDs, "target-az-ids", []string{}, "comma separated list of AWS AZ IDs e.g. usw2-az1,usw2-az2")
	azNatCmd.Flags().StringVar(&azNatArgs.TargetVPCID, "target-vpc-id", "", "vpc to target")
	azNatCmd.Flags().StringVar(&azNatArgs.Region, "region", "", "AWS region to use")
	azNatCmd.Flags().BoolVar(&azNatArgs.Restore, "restore", false, "restores route tables to route to NAT in associated AZs")
	azNatCmd.Flags().BoolVar(&azNatArgs.DryRun, "dry-run", false, "print change but don't replace route")

}
