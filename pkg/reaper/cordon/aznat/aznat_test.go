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
	"flag"
	"io/ioutil"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/keikoproj/governor/pkg/reaper/common"
	"github.com/onsi/gomega"
)

var (
	loggingEnabled bool
)

func init() {
	flag.BoolVar(&loggingEnabled, "logging-enabled", false, "Enable Reaper Logs")
}

type stubEC2 struct {
	ec2iface.EC2API
	gateways              []*ec2.NatGateway
	tables                []*ec2.RouteTable
	subnets               []*ec2.Subnet
	ReplaceRouteCallCount int
}

type FakeGateway struct {
	ID        string
	IsHealthy bool
	SubnetID  string
	VpcID     string
}

type FakeSubnet struct {
	ID                 string
	AvailabilityZoneID string
	VpcID              string
}

type FakeRouteTable struct {
	ID                string
	AssociatedSubnets []string
	VpcID             string
	NatRoutes         []string
}

func _getGateways() []FakeGateway {
	return []FakeGateway{
		_newFakeGateway("nat-1234", "subnet-1234", "vpc-1", true),
		_newFakeGateway("nat-2345", "subnet-2345", "vpc-1", true),
		_newFakeGateway("nat-3456", "subnet-3456", "vpc-1", true),
		_newFakeGateway("nat-4567", "subnet-3456", "vpc-1", false),
		_newFakeGateway("nat-5678", "subnet-1111", "vpc-2", true),
	}
}

func _getSubnets() []FakeSubnet {
	return []FakeSubnet{
		_newFakeSubnet("subnet-1234", "usw2-az1", "vpc-1"),
		_newFakeSubnet("subnet-2345", "usw2-az2", "vpc-1"),
		_newFakeSubnet("subnet-3456", "usw2-az3", "vpc-1"),
		_newFakeSubnet("subnet-6789", "usw2-az1", "vpc-1"),
		_newFakeSubnet("subnet-7890", "usw2-az2", "vpc-1"),
		_newFakeSubnet("subnet-0123", "usw2-az3", "vpc-1"),
		_newFakeSubnet("subnet-4567", "usw2-az1", "vpc-1"),
	}
}

func _getCordonableTables() []FakeRouteTable {
	return []FakeRouteTable{
		_newFakeTable("rtb-1234", "vpc-1", []string{"subnet-6789"}, []string{"nat-1234"}),
		_newFakeTable("rtb-2345", "vpc-1", []string{"subnet-7890"}, []string{"nat-2345"}),
		_newFakeTable("rtb-3456", "vpc-1", []string{"subnet-0123"}, []string{"nat-3456"}),
	}
}

func _getRestorableTables() []FakeRouteTable {
	return []FakeRouteTable{
		_newFakeTable("rtb-1234", "vpc-1", []string{"subnet-6789"}, []string{"nat-3456"}),
		_newFakeTable("rtb-2345", "vpc-1", []string{"subnet-7890"}, []string{"nat-3456"}),
		_newFakeTable("rtb-3456", "vpc-1", []string{"subnet-0123"}, []string{"nat-3456"}),
	}
}

func _newFakeTable(id, vpc string, subnets, routes []string) FakeRouteTable {
	return FakeRouteTable{
		ID:                id,
		VpcID:             vpc,
		AssociatedSubnets: subnets,
		NatRoutes:         routes,
	}
}

func _fakeTables(tb []FakeRouteTable) []*ec2.RouteTable {
	tables := make([]*ec2.RouteTable, 0)
	for _, t := range tb {
		table := &ec2.RouteTable{
			RouteTableId: aws.String(t.ID),
			VpcId:        aws.String(t.VpcID),
		}

		for _, s := range t.AssociatedSubnets {
			table.Associations = append(table.Associations, &ec2.RouteTableAssociation{
				SubnetId: aws.String(s),
			})
		}

		for _, nat := range t.NatRoutes {
			route := &ec2.Route{
				DestinationCidrBlock: aws.String("0.0.0.0/0"),
			}
			if nat != "" {
				route.NatGatewayId = aws.String(nat)
			}

			table.Routes = append(table.Routes, route)

		}

		tables = append(tables, table)
	}
	return tables
}

func _newFakeSubnet(id, az, vpc string) FakeSubnet {
	return FakeSubnet{
		ID:                 id,
		AvailabilityZoneID: az,
		VpcID:              vpc,
	}
}

func _fakeSubnets(sn []FakeSubnet) []*ec2.Subnet {
	subnets := make([]*ec2.Subnet, 0)
	for _, s := range sn {
		subnet := &ec2.Subnet{
			SubnetId:           aws.String(s.ID),
			AvailabilityZoneId: aws.String(s.AvailabilityZoneID),
			VpcId:              aws.String(s.VpcID),
		}
		subnets = append(subnets, subnet)
	}
	return subnets
}

func _newFakeGateway(id, subnet, vpc string, isHealthy bool) FakeGateway {
	return FakeGateway{
		ID:        id,
		SubnetID:  subnet,
		VpcID:     vpc,
		IsHealthy: isHealthy,
	}
}

func _fakeGateways(gws []FakeGateway) []*ec2.NatGateway {
	gateways := make([]*ec2.NatGateway, 0)
	for _, g := range gws {
		gw := &ec2.NatGateway{
			NatGatewayId: aws.String(g.ID),
			SubnetId:     aws.String(g.SubnetID),
			VpcId:        aws.String(g.VpcID),
		}
		if g.IsHealthy {
			gw.State = aws.String(ec2.NatGatewayStateAvailable)
		} else {
			gw.State = aws.String(ec2.NatGatewayStateFailed)
		}
		gateways = append(gateways, gw)
	}
	return gateways
}

func (s *stubEC2) DescribeNatGateways(in *ec2.DescribeNatGatewaysInput) (*ec2.DescribeNatGatewaysOutput, error) {
	return &ec2.DescribeNatGatewaysOutput{
		NatGateways: s.gateways,
	}, nil
}

func (e *stubEC2) DescribeNatGatewaysPages(input *ec2.DescribeNatGatewaysInput, callback func(*ec2.DescribeNatGatewaysOutput, bool) bool) error {
	page, err := e.DescribeNatGateways(input)
	if err != nil {
		return err
	}
	callback(page, false)
	return nil
}

func (s *stubEC2) DescribeRouteTables(in *ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error) {
	return &ec2.DescribeRouteTablesOutput{
		RouteTables: s.tables,
	}, nil
}

func (e *stubEC2) DescribeRouteTablesPages(input *ec2.DescribeRouteTablesInput, callback func(*ec2.DescribeRouteTablesOutput, bool) bool) error {
	page, err := e.DescribeRouteTables(input)
	if err != nil {
		return err
	}
	callback(page, false)
	return nil
}

func (s *stubEC2) DescribeSubnets(in *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
	return &ec2.DescribeSubnetsOutput{
		Subnets: s.subnets,
	}, nil
}

func (e *stubEC2) DescribeSubnetsPages(input *ec2.DescribeSubnetsInput, callback func(*ec2.DescribeSubnetsOutput, bool) bool) error {
	page, err := e.DescribeSubnets(input)
	if err != nil {
		return err
	}
	callback(page, false)
	return nil
}

func (s *stubEC2) ReplaceRoute(in *ec2.ReplaceRouteInput) (*ec2.ReplaceRouteOutput, error) {
	s.ReplaceRouteCallCount++
	return &ec2.ReplaceRouteOutput{}, nil
}

func _fakeCordonContext() *CordonContext {
	if !loggingEnabled {
		log.Out = ioutil.Discard
		common.Log.Out = ioutil.Discard
	}

	return &CordonContext{}
}

type CordonUnitTest struct {
	TestDescription        string
	Context                *CordonContext
	TargetZones            []string
	TargetVPC              string
	Restore                bool
	DryRun                 bool
	Subnets                []FakeSubnet
	Gateways               []FakeGateway
	RouteTables            []FakeRouteTable
	ExpectedReplacedRoutes []Route
	ShouldSucceed          bool
}

func (c *CordonUnitTest) Run(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	c.Context.EC2 = &stubEC2{
		gateways: _fakeGateways(c.Gateways),
		subnets:  _fakeSubnets(c.Subnets),
		tables:   _fakeTables(c.RouteTables),
	}
	err := c.Context.discover(c.TargetVPC)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	err = c.Context.execute(c.TargetZones, c.Restore, c.DryRun)
	if c.ShouldSucceed {
		g.Expect(err).NotTo(gomega.HaveOccurred())
	} else {
		g.Expect(err).To(gomega.HaveOccurred())
	}
	g.Expect(c.Context.ReplacedRoutes).To(gomega.ConsistOf(c.ExpectedReplacedRoutes))

}

func TestBasicCordonSingleZone(t *testing.T) {
	testCase := CordonUnitTest{
		TestDescription: "Test basic cordon operation for a single AZ",
		Context:         _fakeCordonContext(),
		TargetZones:     []string{"usw2-az1"},
		TargetVPC:       "vpc-1",
		Gateways:        _getGateways(),
		Subnets:         _getSubnets(),
		RouteTables:     _getCordonableTables(),
		ExpectedReplacedRoutes: []Route{
			{
				TableID:         "rtb-1234",
				DestinationCIDR: "0.0.0.0/0",
				GatewayID:       "nat-1234",
				NewGateway:      "nat-2345",
			},
		},
		ShouldSucceed: true,
	}
	testCase.Run(t)
}

func TestBasicCordonMultiZone(t *testing.T) {
	testCase := CordonUnitTest{
		TestDescription: "Test basic cordon operation for multiple AZs",
		Context:         _fakeCordonContext(),
		TargetZones:     []string{"usw2-az1", "usw2-az2"},
		TargetVPC:       "vpc-1",
		Gateways:        _getGateways(),
		Subnets:         _getSubnets(),
		RouteTables:     _getCordonableTables(),
		ExpectedReplacedRoutes: []Route{
			{
				TableID:         "rtb-1234",
				DestinationCIDR: "0.0.0.0/0",
				GatewayID:       "nat-1234",
				NewGateway:      "nat-3456",
			},
			{
				TableID:         "rtb-2345",
				DestinationCIDR: "0.0.0.0/0",
				GatewayID:       "nat-2345",
				NewGateway:      "nat-3456",
			},
		},
		ShouldSucceed: true,
	}
	testCase.Run(t)
}

func TestBasicCordonMultiZoneFailed(t *testing.T) {
	testCase := CordonUnitTest{
		TestDescription:        "Test basic cordon operation for all AZs - should fail",
		Context:                _fakeCordonContext(),
		TargetZones:            []string{"usw2-az1", "usw2-az2", "usw2-az3"},
		TargetVPC:              "vpc-1",
		Gateways:               _getGateways(),
		Subnets:                _getSubnets(),
		RouteTables:            _getCordonableTables(),
		ExpectedReplacedRoutes: nil,
		ShouldSucceed:          false,
	}
	testCase.Run(t)
}

func TestBasicUncordonSingleZone(t *testing.T) {
	testCase := CordonUnitTest{
		TestDescription: "Test basic uncordon operation for a single AZ",
		Context:         _fakeCordonContext(),
		TargetZones:     []string{"usw2-az1"},
		TargetVPC:       "vpc-1",
		Restore:         true,
		Gateways:        _getGateways(),
		Subnets:         _getSubnets(),
		RouteTables:     _getRestorableTables(),
		ExpectedReplacedRoutes: []Route{
			{
				TableID:         "rtb-1234",
				DestinationCIDR: "0.0.0.0/0",
				GatewayID:       "nat-3456",
				ZoneID:          "usw2-az1",
				NewGateway:      "nat-1234",
			},
		},
		ShouldSucceed: true,
	}
	testCase.Run(t)
}

func TestBasicUncordonMultiZone(t *testing.T) {
	testCase := CordonUnitTest{
		TestDescription: "Test basic uncordon operation for a multiple AZs",
		Context:         _fakeCordonContext(),
		TargetZones:     []string{"usw2-az1", "usw2-az2"},
		TargetVPC:       "vpc-1",
		Restore:         true,
		Gateways:        _getGateways(),
		Subnets:         _getSubnets(),
		RouteTables:     _getRestorableTables(),
		ExpectedReplacedRoutes: []Route{
			{
				TableID:         "rtb-1234",
				DestinationCIDR: "0.0.0.0/0",
				GatewayID:       "nat-3456",
				ZoneID:          "usw2-az1",
				NewGateway:      "nat-1234",
			},
			{
				TableID:         "rtb-2345",
				DestinationCIDR: "0.0.0.0/0",
				GatewayID:       "nat-3456",
				ZoneID:          "usw2-az2",
				NewGateway:      "nat-2345",
			},
		},
		ShouldSucceed: true,
	}
	testCase.Run(t)
}

func TestBasicUncordonAllZones(t *testing.T) {
	testCase := CordonUnitTest{
		TestDescription: "Test basic uncordon operation for all zones",
		Context:         _fakeCordonContext(),
		TargetZones:     []string{"usw2-az1", "usw2-az2", "usw2-az3"},
		TargetVPC:       "vpc-1",
		Restore:         true,
		Gateways:        _getGateways(),
		Subnets:         _getSubnets(),
		RouteTables:     _getRestorableTables(),
		ExpectedReplacedRoutes: []Route{
			{
				TableID:         "rtb-1234",
				DestinationCIDR: "0.0.0.0/0",
				GatewayID:       "nat-3456",
				ZoneID:          "usw2-az1",
				NewGateway:      "nat-1234",
			},
			{
				TableID:         "rtb-2345",
				DestinationCIDR: "0.0.0.0/0",
				GatewayID:       "nat-3456",
				ZoneID:          "usw2-az2",
				NewGateway:      "nat-2345",
			},
		},
		ShouldSucceed: true,
	}
	testCase.Run(t)
}
