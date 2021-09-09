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
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/keikoproj/governor/pkg/reaper/common"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var log = logrus.New()

func Run(args *Args) error {
	log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	if err := args.validate(); err != nil {
		return errors.Wrap(err, "failed to validate arguments")
	}

	var config aws.Config
	config.Region = &args.Region
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
		Config:            config,
	}))

	ec2Client := ec2.New(sess)

	ctx := CordonContext{
		EC2: ec2Client,
	}

	if err := ctx.discover(args.TargetVPCID); err != nil {
		return errors.Wrap(err, "failed to discover cloud")
	}

	if err := ctx.execute(args.TargetAvailabilityZoneIDs, args.Restore, args.DryRun); err != nil {
		return errors.Wrap(err, "failed to execute operation")
	}

	return nil
}

func (c *CordonContext) execute(targetZones []string, restore, dryRun bool) error {
	// get cordoned routes
	var (
		routes     []Route
		zoneMap    = c.zoneGateways()
		gwStatuses GatewayStatuses
	)

	if restore {
		log.Infof("running route restore operation on zones: %+v, dry-run: %t", targetZones, dryRun)
		routes = c.uncordonedRoutes(targetZones)
	} else {
		gwStatuses = c.gatewayStatuses(targetZones)
		if len(gwStatuses.Active) == 0 {
			log.Errorf("could not find active nat-gateways excluding %v, cannot uncordon all zones", targetZones)
			return errors.Errorf("no active nat-gateways found")
		}

		log.Infof("running route cordon operation on zones: %+v, dry-run: %t", targetZones, dryRun)
		routes = c.cordonedRoutes(gwStatuses)
	}

	if len(routes) == 0 {
		log.Info("could not find routes to cordon/uncordon")
	}

	// replace gateway for cordoned routes
	for _, r := range routes {
		if restore {
			if gws, ok := zoneMap[r.ZoneID]; ok {
				r.NewGateway = gws[0]
			}
		} else {
			r.NewGateway = gwStatuses.Active[0]
		}
		if dryRun {
			log.Warnf("--dry-run flag is set, will skip replacement of route table %+v with new gateway %v\n", r.TableID, r.NewGateway)
			continue
		}
		err := c.replaceGatewayRoute(r)
		if err != nil {
			return err
		}
	}
	log.Infof("execution completed, replaced %v routes", c.ReplacedRoutes)
	return nil
}

func (c *CordonContext) discover(vpc string) error {

	filters := []*ec2.Filter{
		{
			Name:   aws.String("vpc-id"),
			Values: aws.StringSlice([]string{vpc}),
		},
	}

	err := c.EC2.DescribeSubnetsPages(
		&ec2.DescribeSubnetsInput{Filters: filters}, func(page *ec2.DescribeSubnetsOutput, lastPage bool) bool {
			c.Subnets = append(c.Subnets, page.Subnets...)
			return page.NextToken != nil
		},
	)
	if err != nil {
		return err
	}

	err = c.EC2.DescribeRouteTablesPages(
		&ec2.DescribeRouteTablesInput{Filters: filters}, func(page *ec2.DescribeRouteTablesOutput, lastPage bool) bool {
			c.RouteTables = append(c.RouteTables, page.RouteTables...)
			return page.NextToken != nil
		},
	)
	if err != nil {
		return err
	}

	err = c.EC2.DescribeNatGatewaysPages(
		&ec2.DescribeNatGatewaysInput{Filter: filters}, func(page *ec2.DescribeNatGatewaysOutput, lastPage bool) bool {
			c.Gateways = append(c.Gateways, page.NatGateways...)
			return page.NextToken != nil
		},
	)
	if err != nil {
		return err
	}

	zoneGateways := c.zoneGateways()

	associations := make([]SubnetAssociation, 0)
	for _, s := range c.Subnets {
		var gateways []string
		zone := aws.StringValue(s.AvailabilityZoneId)
		if gw, ok := zoneGateways[zone]; ok {
			gateways = gw
		}
		subnetId := aws.StringValue(s.SubnetId)
		association := SubnetAssociation{
			SubnetID:   subnetId,
			ZoneID:     zone,
			GatewayIDs: gateways,
		}
		associations = append(associations, association)
	}
	c.SubnetAssociations = associations
	return nil
}

func (c *CordonContext) replaceGatewayRoute(route Route) error {
	log.Infof("replacing route-table entry in table %v: %v->%v to %v->%v", route.TableID, route.DestinationCIDR, route.GatewayID, route.DestinationCIDR, route.NewGateway)
	_, err := c.EC2.ReplaceRoute(&ec2.ReplaceRouteInput{
		RouteTableId:         aws.String(route.TableID),
		DestinationCidrBlock: aws.String(route.DestinationCIDR),
		NatGatewayId:         aws.String(route.NewGateway),
	})
	if err != nil {
		return err
	}
	c.ReplacedRoutes = append(c.ReplacedRoutes, route)
	return nil
}

func (c *CordonContext) uncordonedRoutes(targetZones []string) []Route {
	// find routes to different zones
	routes := make([]Route, 0)

	for _, r := range c.RouteTables {
		tableId := aws.StringValue(r.RouteTableId)
		zones := c.routeTableZones(r)
		if len(zones) != 1 {
			log.Infof("skipping route table '%v' since it is associated with multiple zones", tableId)
			continue
		}
		if !common.StringSliceContains(targetZones, zones[0]) {
			continue
		}
		rts := getNatRoutes(r)
		for _, rt := range rts {
			destination := aws.StringValue(rt.DestinationCidrBlock)
			routeNat := aws.StringValue(rt.NatGatewayId)
			gwSubnet := c.gatewaySubnet(routeNat)
			gwSubnetAssoc := c.getSubnetAssociation(gwSubnet)
			if !strings.EqualFold(zones[0], gwSubnetAssoc.ZoneID) {
				routes = append(routes, Route{
					TableID:         tableId,
					DestinationCIDR: destination,
					GatewayID:       routeNat,
					ZoneID:          zones[0],
				})
			}
		}
	}
	return routes
}

func (c *CordonContext) cordonedRoutes(gwStatuses GatewayStatuses) []Route {
	routes := make([]Route, 0)
	for _, r := range c.RouteTables {
		rts := getNatRoutes(r)
		for _, rt := range rts {
			gateway := aws.StringValue(rt.NatGatewayId)
			if common.StringSliceContains(gwStatuses.Cordoned, gateway) {
				routes = append(routes, Route{
					TableID:         aws.StringValue(r.RouteTableId),
					DestinationCIDR: aws.StringValue(rt.DestinationCidrBlock),
					GatewayID:       gateway,
				})
			}
		}
	}
	return routes
}

func (c *CordonContext) zoneGateways() map[string][]string {
	subnetZones := make(map[string]string)
	for _, s := range c.Subnets {
		zone := aws.StringValue(s.AvailabilityZoneId)
		subnet := aws.StringValue(s.SubnetId)
		subnetZones[subnet] = zone
	}

	zoneGateways := make(map[string][]string)
	for _, g := range c.Gateways {
		gwSubnet := aws.StringValue(g.SubnetId)
		gw := aws.StringValue(g.NatGatewayId)
		state := aws.StringValue(g.State)
		if !strings.EqualFold(state, ec2.NatGatewayStateAvailable) {
			continue
		}
		if zone, ok := subnetZones[gwSubnet]; ok {
			zoneGateways[zone] = append(zoneGateways[zone], gw)
		}
	}

	return zoneGateways
}

func getNatRoutes(rtb *ec2.RouteTable) []*ec2.Route {
	routes := make([]*ec2.Route, 0)
	for _, r := range rtb.Routes {
		if r.NatGatewayId != nil {
			routes = append(routes, r)
		}
	}
	return routes
}
