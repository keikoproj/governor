# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]
Bleeding-edge development, not yet released

## [v0.3.4] - 2022-06-03

### What's Changed
* Fix PDB Reaper metrics for the following values by @shaoxt in https://github.com/keikoproj/governor/pull/65
* cross-compile image & update go version by @eytan-avisror in https://github.com/keikoproj/governor/pull/68

**Full Changelog**: https://github.com/keikoproj/governor/compare/0.3.3...0.3.4


## [v0.3.3] - 2022-01-26

### What's Changed
* Basic Metrics for Pod/Node Reaper by @eytan-avisror in https://github.com/keikoproj/governor/pull/64

**Full Changelog**: https://github.com/keikoproj/governor/compare/0.3.2...0.3.3


## [v0.3.2] - 2021-12-21

### What's Changed
* PDB Reaper Metrics by @shaoxt in https://github.com/keikoproj/governor/pull/63

**Full Changelog**: https://github.com/keikoproj/governor/compare/0.3.1...0.3.2


## [v0.3.1] - 2021-11-30

### Added 
* Add flag to keep node cordoned for reap failures (#60)
* Add not-ready reap logic and unit test (#58 ) 
* Add NAT AZ Cordon tool (#56)
* Helm Charts (#55)

### Fixed
* Pull region from node labels if well known label is present (#59)

### What's Changed
* adding helm chart by @ssandy83 in https://github.com/keikoproj/governor/pull/55
* Add NAT AZ Cordon tool by @eytan-avisror in https://github.com/keikoproj/governor/pull/56
* add not-ready reap logic and unit test by @sahilbadla in https://github.com/keikoproj/governor/pull/58
* Pull region from node labels if well known label is present by @backjo in https://github.com/keikoproj/governor/pull/59
* Add flag to keep node cordoned for reap failures by @shailshah9 in https://github.com/keikoproj/governor/pull/60

### New Contributors
* @ssandy83 made their first contribution in https://github.com/keikoproj/governor/pull/55
* @sahilbadla made their first contribution in https://github.com/keikoproj/governor/pull/58
* @backjo made their first contribution in https://github.com/keikoproj/governor/pull/59
* @shailshah9 made their first contribution in https://github.com/keikoproj/governor/pull/60

**Full Changelog**: https://github.com/keikoproj/governor/compare/0.3.0...0.3.1


## [v0.3.0] - 2021-07-19

### Added 
* PDB Reaper (#52)
* Log node drain command output as info log (#48)
* Add drain timeout flag for node reaper (#45)
* Reconsider unreapable nodes after specified time (#44)

### Changed
* Move to github actions (#53)

### Fixed
* Avoid ASG Validation on Unjoined nodes (#54)

### What's Changed
* Reconsider unreapable nodes after specified time by @viveksyngh in https://github.com/keikoproj/governor/pull/44
* Add drain timeout flag for node reaper by @viveksyngh in https://github.com/keikoproj/governor/pull/45
* Log node drain command output as info log by @viveksyngh in https://github.com/keikoproj/governor/pull/48
* move to github actions by @eytan-avisror in https://github.com/keikoproj/governor/pull/53
* PDB Reaper by @eytan-avisror in https://github.com/keikoproj/governor/pull/52
* Refactor ReapableInstances to avoid ASG Validation selectively  by @eytan-avisror in https://github.com/keikoproj/governor/pull/54

### New Contributors
* @viveksyngh made their first contribution in https://github.com/keikoproj/governor/pull/44

**Full Changelog**: https://github.com/keikoproj/governor/compare/0.2.6...0.3.0


## [v0.2.6] - 2020-08-12

### What's Changed
* Update aws-sdk-go to support IRSA by @kianjones4 in https://github.com/keikoproj/governor/pull/37

### New Contributors
* @kianjones4 made their first contribution in https://github.com/keikoproj/governor/pull/37

**Full Changelog**: https://github.com/keikoproj/governor/compare/0.2.5...0.2.6


## [v0.2.5] - 2020-07-28

### Added
Node-Reaper: Skip Label Support -- allows ignoring certain conditions according to node labels (#32)
Node-Reaper: Support removal of tainted nodes (#35)

### Fixed
Pod-Reaper: Failed pod removal - fixed case for Evicted pods. (#34)

### What's Changed
* Nodereaper skip label by @pyieh in https://github.com/keikoproj/governor/pull/32
* reap evicted pods by @eytan-avisror in https://github.com/keikoproj/governor/pull/34
* Reap tainted nodes by @eytan-avisror in https://github.com/keikoproj/governor/pull/35

### New Contributors
* @pyieh made their first contribution in https://github.com/keikoproj/governor/pull/32

**Full Changelog**: https://github.com/keikoproj/governor/compare/0.2.4...0.2.5


## [v0.2.4] - 2020-04-16

### Added
Pod-Reaper: Support clean up of completed/failed pods (#27)
Pod-Reaper: Namespace exclusion by annotation(#28)

**NOTE**: This release requires an RBAC change. See [Required RBAC Permissions](https://github.com/keikoproj/governor/tree/master/pkg/reaper#required-rbac-permissions) for more information.

### What's Changed
* Failed / Completed pod support by @eytan-avisror in https://github.com/keikoproj/governor/pull/29

**Full Changelog**: https://github.com/keikoproj/governor/compare/0.2.3...0.2.4


## [v0.2.3] - 2020-02-19

### What's Changed
* Improved implementation for annotating terminating nodes by @eytan-avisror in https://github.com/keikoproj/governor/pull/26 (#25)

**Full Changelog**: https://github.com/keikoproj/governor/compare/0.2.2...0.2.3


## [v0.2.2] - 2020-02-18

### Added
* Annotate terminating nodes (#25)
* Migrate to modules (#19)

### What's Changed
* Migrate to modules by @matt0x6F in https://github.com/keikoproj/governor/pull/19
* Increase test coverage(#6) by @pratyushprakash in https://github.com/keikoproj/governor/pull/22
* Annotate terminating nodes by @eytan-avisror in https://github.com/keikoproj/governor/pull/25

### New Contributors
* @matt0x6F made their first contribution in https://github.com/keikoproj/governor/pull/19
* @pratyushprakash made their first contribution in https://github.com/keikoproj/governor/pull/22

**Full Changelog**: https://github.com/keikoproj/governor/compare/0.2.1...0.2.2


## [v0.2.1] - 2019-09-30

### Fixed

* Bug: Unjoined nodes discovers already terminated instances and does not honor threshold (#16)

### What's Changed
* Fix unjoined instance scanning by @eytan-avisror in https://github.com/keikoproj/governor/pull/17

**Full Changelog**: https://github.com/keikoproj/governor/compare/0.2.0...0.2.1


## [v0.2.0] - 2019-09-30

### What's Changed
* Node Reaper: Ghost/Unjoined Nodes by @eytan-avisror in https://github.com/keikoproj/governor/pull/13
* Prepare Release 0.2.0 by @eytan-avisror in https://github.com/keikoproj/governor/pull/15

**Full Changelog**: https://github.com/keikoproj/governor/compare/0.1.0...0.2.0


## [v0.1.0] - 2019-08-28

The initial alpha release of governor

### What's Changed
* CI: Travis Integration by @eytan-avisror in https://github.com/keikoproj/governor/pull/2
* CI: Fix travis integration by @eytan-avisror in https://github.com/keikoproj/governor/pull/3
* Badges + Gofmt & Lint by @eytan-avisror in https://github.com/keikoproj/governor/pull/4
* Add codeowners by @eytan-avisror in https://github.com/keikoproj/governor/pull/8
* Rename Org by @eytan-avisror in https://github.com/keikoproj/governor/pull/10

**Full Changelog**: https://github.com/keikoproj/governor/commits/0.1.0