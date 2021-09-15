# governor
[![Build Status](https://travis-ci.org/keikoproj/governor.svg?branch=master)](https://travis-ci.org/keikoproj/governor)
[![codecov](https://codecov.io/gh/keikoproj/governor/branch/master/graph/badge.svg)](https://codecov.io/gh/keikoproj/governor)
[![Go Report Card](https://goreportcard.com/badge/github.com/keikoproj/governor)](https://goreportcard.com/report/github.com/keikoproj/governor)
> A collection of cluster reliability tools built for Kubernetes

Governor is a collection of tools for improving the stability of the large Kubernetes clusters as a single Docker image.

Two common problems observed in large Kubernetes clusters are:

1. Node failure due to underlying cloud provider issues.
2. Pods being stuck in "Terminating" state and unable to be cleaned up.

**node-reaper** provides the capability for worker nodes to be force terminated so that replacement ones come up.
**pod-reaper** does a force termination of pods stuck in Terminating state for a certain amount of time.

In some cases, on multi-tenant platforms, users can own PDBs which block node rotation/drain if they are misconfigured, pods are in crashloop backoff, or if multiple PDBs are targeting the same pods.

**pdb-reaper** provides the capability for detecting PDBs in these conditions, and deleting them. This is especially useful for pre-production environments where pods might be left around in crashloop, or misconfigured PDBs may exist blocking node drains.

When pdb-reaper deletes PDBs, it does **NOT** recreate them, this is useful when GitOps is used. We recommend using pdb-reaper in non-production environemnts or use the `--dry-run` flag to have event publishing without deletion of PDBs.

There are many corner-cases where deleting PDBs might be dangerous, please consider such cases when using pdb-reaper.

**cordon** provides capabilities around cordoning specific data paths in AWS, for example excluding a specific NAT Gatway in case of AZ failure.

## Usage

Assuming an AWS-hosted running kubernetes cluster:

```sh
kubectl create namespace governor

# Using a CronJob
kubectl apply -n governor -f https://raw.githubusercontent.com/keikoproj/governor/master/examples/node-reaper.yaml

kubectl apply -n governor -f https://raw.githubusercontent.com/keikoproj/governor/master/examples/pod-reaper.yaml

kubectl apply -n governor -f https://raw.githubusercontent.com/keikoproj/governor/master/examples/pdb-reaper.yaml
```

### Available Packages

| Package | Description | Docs
| :--- | :--- | :---: |
| node-reaper | terminates nodes in scaling groups | [node-reaper](pkg/reaper/README.md#node-reaper) |
| pod-reaper | force terminates stuck pods | [pod-reaper](pkg/reaper/README.md#pod-reaper) |
| pdb-reaper | deletes blocking PDBs | [pdb-reaper](pkg/reaper/README.md#pdb-reaper) |
| cordon | helps with cordoning AWS data paths | [cordon](pkg/cordon/README.md#cordon)

## Release History

Please see [CHANGELOG.md](.github/CHANGELOG.md).

## ❤ Contributing ❤

Please see [CONTRIBUTING.md](.github/CONTRIBUTING.md).

## Developer Guide

Please see [DEVELOPER.md](.github/DEVELOPER.md).
