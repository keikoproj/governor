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

package aznat

import (
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/keikoproj/governor/pkg/reaper/common"
	"github.com/pkg/errors"
)

type CordonContext struct {
	EC2                ec2iface.EC2API
	Subnets            []*ec2.Subnet
	RouteTables        []*ec2.RouteTable
	Gateways           []*ec2.NatGateway
	SubnetAssociations []SubnetAssociation
	ReplacedRoutes     []Route
}

type SubnetAssociation struct {
	SubnetID   string
	GatewayIDs []string
	ZoneID     string
}

type GatewayStatuses struct {
	Active   []string
	Cordoned []string
}

type Route struct {
	TableID         string
	DestinationCIDR string
	GatewayID       string
	ZoneID          string
	NewGateway      string
}

type Args struct {
	TargetAvailabilityZoneIDs []string
	TargetVPCID               string
	Region                    string
	Restore                   bool
	DryRun                    bool
}

var (
	ShorthandRegions = []string{"use1", "use2", "usw1", "usw2", "usgw2", "cac1", "ew1", "ew2", "ec1", "apse1", "apse2", "aps1", "apne1", "apne2", "sae1", "cn1"}
	Regions          = []string{
		"us-east-1",
		"us-east-2",
		"us-west-1",
		"us-west-2",
		"us-gov-west-1",
		"ca-central-1",
		"eu-west-1",
		"eu-west-2",
		"eu-central-1",
		"ap-southeast-1",
		"ap-southeast-2",
		"ap-south-1",
		"ap-northeast-1",
		"ap-northeast-2",
		"sa-east-1",
		"cn-north-1",
	}
)

func (a *Args) validate() error {
	if len(a.TargetAvailabilityZoneIDs) < 1 {
		return errors.Errorf("--target-az-ids: must provide atleast one target availability zone")
	}
	for _, az := range a.TargetAvailabilityZoneIDs {
		azSplit := strings.Split(az, "-")
		if len(azSplit) != 2 {
			return errors.Errorf("--target-az-ids: bad az '%v' provided", az)
		}
		if !common.StringSliceContains(ShorthandRegions, azSplit[0]) {
			return errors.Errorf("--target-az-ids: bad az '%v' provided, region must be one of %v", az, ShorthandRegions)
		}

		if match, _ := regexp.MatchString("az[1-9]", azSplit[1]); !match {
			return errors.Errorf("--target-az-ids: bad az '%v' provided, az must be in the form of region-az, e.g. usw2-az1", az)
		}
	}

	if a.TargetVPCID == "" {
		return errors.Errorf("--target-vpc-id: vpc-id is required")
	}

	if match, _ := regexp.MatchString("vpc-\\w{17}$", a.TargetVPCID); !match {
		return errors.Errorf("--target-vpc-id: bad vpc-id '%v' provided", a.TargetVPCID)
	}

	if a.Region == "" {
		return errors.Errorf("--region: region is required")
	}

	if !common.StringSliceContains(Regions, a.Region) {
		return errors.Errorf("--region: unknown region '%v' provided", a.Region)
	}

	return nil
}

func (c *CordonContext) getSubnetAssociation(subnetId string) SubnetAssociation {
	for _, assoc := range c.SubnetAssociations {
		if strings.EqualFold(subnetId, assoc.SubnetID) {
			return assoc
		}
	}
	return SubnetAssociation{}
}

func (c *CordonContext) gatewaySubnet(gatewayId string) string {
	for _, g := range c.Gateways {
		subnet := aws.StringValue(g.SubnetId)
		gateway := aws.StringValue(g.NatGatewayId)
		state := aws.StringValue(g.State)
		if !strings.EqualFold(state, ec2.NatGatewayStateAvailable) {
			continue
		}
		if strings.EqualFold(gateway, gatewayId) {
			return subnet
		}
	}
	return ""
}

func (c *CordonContext) gatewayStatuses(targetZones []string) GatewayStatuses {
	active := make([]string, 0)
	cordoned := make([]string, 0)
	for _, s := range c.SubnetAssociations {
		if common.StringSliceContains(targetZones, s.ZoneID) {
			for _, gw := range s.GatewayIDs {
				if !common.StringSliceContains(cordoned, gw) {
					cordoned = append(cordoned, gw)
				}
			}
		} else {
			for _, gw := range s.GatewayIDs {
				if !common.StringSliceContains(active, gw) {
					active = append(active, gw)
				}
			}
		}
	}
	return GatewayStatuses{
		Active:   active,
		Cordoned: cordoned,
	}
}

func (c *CordonContext) routeTableZones(table *ec2.RouteTable) []string {
	associations := make([]string, 0)
	for _, assoc := range table.Associations {
		subnetId := aws.StringValue(assoc.SubnetId)
		assoc := c.getSubnetAssociation(subnetId)
		if assoc.ZoneID != "" && !common.StringSliceContains(associations, assoc.ZoneID) {
			associations = append(associations, assoc.ZoneID)
		}
	}

	return associations
}
