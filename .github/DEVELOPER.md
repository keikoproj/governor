# Development reference

This document will walk you through setting up a basic testing environment, running unit tests and testing changes locally.
This document will also assume a running Kubernetes cluster on AWS.

## Running locally

You can run the `main.go` file with the appropriate command line arguments to invoke a governor package.
The `--local-mode` flag tells governor to use a local kubeconfig instead of `InClusterAuth`, if your kubeconfig is not in it's default location you can point to it's path using `--kubeconfig`.

### Example

```bash
$ go run cmd/governor/governor.go reap pod \
--local-mode \
--reap-after 6 \
--dry-run \
--soft-reap

INFO[0000] starting cluster external auth
INFO[0000] kubeconfig: ~/.kube/config
INFO[0000] target: https://cluster-endpoint
INFO[2019-07-23T13:27:02-07:00] starting scan cycle
INFO[2019-07-23T13:27:04-07:00] found 19 pods
INFO[2019-07-23T13:27:04-07:00] no terminating pods found
```

## Running unit tests

Using the `Makefile` you can run basic unit tests.

### Example

```bash
$ make test
go test -v ./...
?       github.com/keikoproj/governor/cmd/governor   [no test files]
?       github.com/keikoproj/governor/cmd/governor/app   [no test files]
?       github.com/keikoproj/governor/pkg/reaper/common  [no test files]
=== RUN   TestGetUnreadyNodesPositive
--- PASS: TestGetUnreadyNodesPositive (0.00s)
=== RUN   TestGetUnreadyNodesNegative
--- PASS: TestGetUnreadyNodesNegative (0.00s)
=== RUN   TestReapOldPositive
--- PASS: TestReapOldPositive (0.03s)
=== RUN   TestReapOldNegative
--- PASS: TestReapOldNegative (0.00s)
=== RUN   TestReapOldDisabled
--- PASS: TestReapOldDisabled (0.00s)
=== RUN   TestReapOldSelfEviction
--- PASS: TestReapOldSelfEviction (0.00s)
=== RUN   TestReapUnknownPositive
--- PASS: TestReapUnknownPositive (0.00s)
=== RUN   TestReapUnknownNegative
--- PASS: TestReapUnknownNegative (0.00s)
=== RUN   TestReapUnreadyPositive
--- PASS: TestReapUnreadyPositive (0.00s)
=== RUN   TestReapUnreadyNegative
--- PASS: TestReapUnreadyNegative (0.00s)
=== RUN   TestFlapDetectionPositive
--- PASS: TestFlapDetectionPositive (0.01s)
=== RUN   TestFlapDetectionNegative
--- PASS: TestFlapDetectionNegative (0.00s)
=== RUN   TestAsgValidationPositive
--- PASS: TestAsgValidationPositive (0.00s)
=== RUN   TestAsgValidationNegative
--- PASS: TestAsgValidationNegative (0.01s)
=== RUN   TestSoftReapPositive
--- PASS: TestSoftReapPositive (0.00s)
=== RUN   TestSoftReapNegative
--- PASS: TestSoftReapNegative (0.01s)
=== RUN   TestReapThrottleWaiter
--- PASS: TestReapThrottleWaiter (2.00s)
=== RUN   TestAgeReapThrottleWaiter
--- PASS: TestAgeReapThrottleWaiter (2.02s)
=== RUN   TestDryRun
--- PASS: TestDryRun (0.00s)
=== RUN   TestKillOldMasterMinMasters
--- PASS: TestKillOldMasterMinMasters (0.01s)
=== RUN   TestMaxKill
--- PASS: TestMaxKill (0.00s)
=== RUN   TestProviderIDParser
--- PASS: TestProviderIDParser (0.00s)
PASS
coverage: 60.3% of statements
ok      github.com/keikoproj/governor/pkg/reaper/nodereaper  4.396s  coverage: 60.3% of statements
=== RUN   TestDeriveStatePositive
--- PASS: TestDeriveStatePositive (0.00s)
=== RUN   TestDeriveStateNegative
--- PASS: TestDeriveStateNegative (0.00s)
=== RUN   TestDeriveTimeReapablePositive
--- PASS: TestDeriveTimeReapablePositive (0.00s)
=== RUN   TestDeriveTimeReapableNegative
--- PASS: TestDeriveTimeReapableNegative (0.00s)
=== RUN   TestDryRunPositive
--- PASS: TestDryRunPositive (0.00s)
=== RUN   TestSoftReapPositive
--- PASS: TestSoftReapPositive (0.00s)
=== RUN   TestSoftReapNegative
--- PASS: TestSoftReapNegative (0.00s)
=== RUN   TestValidateArgumentsPositive
--- PASS: TestValidateArgumentsPositive (0.00s)
=== RUN   TestValidateArgumentsNegative
--- PASS: TestValidateArgumentsNegative (0.00s)
coverage: 65.8% of statements
ok      github.com/keikoproj/governor/pkg/reaper/podreaper   0.377s  coverage: 65.8% of statements
```

You can use `make vtest` to run test with verbose logging, you can also run `make coverage` to generate a coverage report.
