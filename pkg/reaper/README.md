## Pod Reaper

### What is a reapable pod

A pod is determined to be 'reapable' by reaper in the following conditions:

- It is terminating (by virtue of `Metadata.DeletionTimestamp != IsZero()`)

- The difference between the [<sup>1</sup>](#1)`adjusted deletion timestamp` and time of reaping is greater than the configurable thresholds (`--reap-after` flag)

- By default, reaper will use `Soft Reaping` which means it will not reap pods that still have containers running, this can be turned off by setting `--soft-reap=false`

<a class="anchor" id="1">1. </a>Adjusted deletion timestamp is calculated by `sum(Metadata.DeletionTimestamp - Metadata.DeletionGracePeriodSeconds - Spec.TerminationGracePeriodSeconds)`

#### Completed and Failed pods

By using the flags `--reap-completed` and `--reap-failed` you can allow pod-reaper to delete pods marked completed or failed, while the respective flags `--reap-completed-after` and `--reap-failed-after` will set the time threshold for the deletion.

This is helpful when wanting to automatically clean up these pods across your cluster to avoid load on API Server by controllers that list / operate on pods.

A pod is determined to be completed / failed by it's `Status.Phase`, and the threshold is calculated by looking at when the last container exited. so if you use the default thresholds, these pods will be considered reapable 4 hrs after the last container exited (given the phase of the pod is completed/failed).

#### Namespace level control

You may want to disable certain features for certain namespaces, you can annotate your namespaces accordingly to control which features are active. Use the package flags to control options globally (by default pod-reaper run on all namespaces).

##### Namespace annotations

| Annotation Key | Annotation Value | Action
|---|:---:|:---:|
| governor.keikoproj.io/disable-pod-reaper | "true" | disable all features
| governor.keikoproj.io/disable-completed-pod-reap | "true" | disable completed pod reaping
| governor.keikoproj.io/disable-completed-pod-reap | "true" | disable failed pod reaping
| governor.keikoproj.io/disable-stuck-pod-reap | "true" | disable terminating/stuck pod reaping

### Usage

```text
Usage:
  governor reap pod [flags]

Flags:
      --dry-run                      Will not terminate pods
  -h, --help                         help for pod
      --kubeconfig string            Absolute path to the kubeconfig file
      --local-mode                   Use cluster external auth
      --reap-after float             Reaping threshold in minutes (default 10)
      --reap-completed               Delete pods in completed phase
      --reap-completed-after float   Reaping threshold in minutes for completed pods (default 240)
      --reap-failed                  Delete pods in failed phase
      --reap-failed-after float      Reaping threshold in minutes for failed pods (default 240)
      --soft-reap                    Will not terminate pods with running containers (default true)
```

#### Example

There are three pods 'stuck' in terminating state, `pod-1` for 10m, `pod-2` for 8m and `pod-3` for 3m.
All containers are successfully terminated besides `pod-2`'s.

```bash
$ kubectl get pods --all-namespaces | grep Terminating
NAME                                 READY     STATUS        RESTARTS   AGE
pod-1                                0/1       Terminating   0          10m
pod-2                                1/1       Terminating   0          8m
pod-3                                0/1       Terminating   0          3m
```

Reaper's default configuration will only cause `pod-1` to be reaped, as `pod-2` does not meet the `--soft-reap` condition of zero running containers, and `pod-3` does not meet the `--reap-after` threshold of 10 minutes.

```bash
# Run reaper in localmode (out of cluster using a kubeconfig file)
# Set reap threshold to 6 minutes (--reap-after)
# Don't really reap (--dry-run)
# Only reap if containers are dead (--soft-reap)

$ go run cmd/governor/governor.go reap pod \
--local-mode \
--reap-after 6 \
--dry-run \
--soft-reap
```

Before reaping, reaper will dump the pod spec to log.

### Required RBAC Permissions

```yaml
rules:
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "delete", "list"]
- apiGroups: [""]
  resources: ["namespaces"]
  verbs: ["list"]
```

## Node Reaper

### What is a reapable node

A node is determined to be 'reapable' by reaper in the following conditions:

- It is in NotReady or Unknown state (by virtue of node's readiness condition)

- The difference between the `lastTransitionTime` and time of reaping is greater than the configurable thresholds (`--reap-after` flag)

- By default, reaper will use soft reaping which means it will not reap nodes that still have pods running (by virtue of their own readiness condition), this can be turned off by setting `--soft-reap=false`

### Usage

```text
Usage:
  governor reap node [flags]

Flags:
      --asg-validation                     Validate AutoScalingGroup's Min and Desired match before reaping (default true)
      --dry-run                            Will not terminate node instances
      --flap-count int32                   Only reap instances which have flapped atleast N times over the last hour (default 5)
  -h, --help                               help for node
      --kubeconfig string                  Absolute path to the kubeconfig file
      --kubectl string                     Absolute path to the kubectl binary (default "/usr/local/bin/kubectl")
      --local-mode                         Use cluster external auth
      --max-kill-nodes int                 Kill up to N nodes per job run, considering throttle wait times (default 3)
      --control-plane-node-count           Expected number of control plane nodes (default 3)
      --node-healthcheck-interval          Time (in seconds) to wait between node healthchecks (default 10)
      --node-healthcheck-timeout           Time (in seconds) to wait before node healthcheck timeout (default 1800)
      --cluster-id                         Unique cluster identifier; used for node reaper locks. Only required if --locks-table-name is set. 
      --locks-table-name                   DynamoDB table name for storing the lock objects (default none)
      --reap-after float                   Reaping threshold in minutes (default 10)
      --reap-flappy                        Terminate nodes which have flappy readiness (default true)
      --reap-old                           Terminate nodes older than --reap-old-threshold days
      --reap-old-threshold-minutes int32   Reap N minute old nodes (default 30)
      --reap-old-throttle int              Post terminate wait in seconds for old nodes (default 300)
      --reap-throttle int                  Post terminate wait in seconds for unhealthy nodes(default 300)
      --reap-unknown                       Terminate nodes where State = Unknown (default true)
      --reap-unready                       Terminate nodes where State = NotReady (default true)
      --region string                      AWS Region to operate in
      --soft-reap                          Will not terminate nodes with running pods (default true)
```

#### Locking node reaper

It is possible to coordinate multiple instances of `node-reaper` to avoid reaping more than one control plane node at a time. This may be desirable in some situations, notably the one
described in [kops#13686](https://github.com/kubernetes/kops/issues/13686).

The locking mechanism utilizes a DynamoDB table accessible to all instances of `node-reaper`. Expected table attribute is

```hcl
attribute {
  name = "LockType"
  type = "S"
}
```

where `LockType` is the hash key.

#### Example

There are two nodes in a non ready state, `node-1` is NotReady for 15m, `node-2` has stopped reporting and is Unknown for 10m `node-3` is NotReady for 20m but still has active pods.

```bash
$ kubectl get nodes | grep NotReady
NAME        STATUS       ROLES     AGE       VERSION
node-1      NotReady     node      1d        v1.12.3
node-2      Unknown      node      1d        v1.12.3
node-3      NotReady     node      1d        v1.12.3
```

Reaper's default configuration will cause only `node-1` & `node-2` to be reaped, as `node-3` does not meet the `--soft-reap` threshold of zero active pods.

```bash
# Run reaper in localmode (out of cluster using a kubeconfig file)
# Set AWS region
# Set reap threshold to 5 minutes (--reap-after)
# Don't really reap (--dry-run)
# Only reap if pods are dead (--soft-reap)
# Reap nodes in Unknown or NotReady states (--reap-unknown, --reap-unready)

$ go run cmd/governor/governor.go reap node \
--local-mode \
--region us-west-2 \
--reap-after 5 \
--dry-run \
--soft-reap \
--reap-unknown \
--reap-unready
```

Before reaping, reaper will dump the pod spec to log.

```text
INFO[0004] found 3 nodes and 25 pods
INFO[0004] node node-1 is not ready
INFO[0004] node node-2 is not ready
INFO[0004] node node-2 is not ready
INFO[0004] inspecting pods assigned to node-1
INFO[0004] node node-1 is reapable !! State = Unknown, diff: 15.00/10
INFO[0004] inspecting pods assigned to node-2
INFO[0004] node node-2 is reapable !! State = NotReady, diff: 10.00/10
INFO[0004] inspecting pods assigned to node-2
INFO[0004] node node-2 is not reapable, running pods detected
INFO[0004] reaping node node-1 -> i-1a1a12a1a121a12121
INFO[0004] node dump: {"metadata":{"name":"node-1","creationTimestamp":null},"spec":{"providerID":"aws:///us-west-2a/i-1a1a12a1a121a12121"},"status":{"conditions":[{"type":"Ready","status":"Unknown","lastHeartbeatTime":null,"lastTransitionTime":"2019-01-27T19:09:29Z","reason":"NodeStatusUnknown","message":"Kubelet stopped posting node status."}],"daemonEndpoints":{"kubeletEndpoint":{"Port":0}},"nodeInfo":{"machineID":"","systemUUID":"","bootID":"","kernelVersion":"","osImage":"","containerRuntimeVersion":"","kubeletVersion":"","kubeProxyVersion":"","operatingSystem":"","architecture":""}}}
INFO[0004] reaping node node-2 -> i-1b1b12b1b121b12121
WARN[0004] dry run is on, instance not terminated
INFO[0004] node dump: {"metadata":{"name":"node-2","creationTimestamp":null},"spec":{"providerID":"aws:///us-west-2a/i-1b1b12b1b121b12121"},"status":{"conditions":[{"type":"Ready","status":"False","lastHeartbeatTime":null,"lastTransitionTime":"2019-01-27T19:09:29Z","reason":"KubeletNotReady","message":"PLEG is not healthy: pleg was last seen active 9h14m3.5466392s ago; threshold is 3m0"}],"daemonEndpoints":{"kubeletEndpoint":{"Port":0}},"nodeInfo":{"machineID":"","systemUUID":"","bootID":"","kernelVersion":"","osImage":"","containerRuntimeVersion":"","kubeletVersion":"","kubeProxyVersion":"","operatingSystem":"","architecture":""}}}
WARN[0004] dry run is on, instance not terminated
INFO[0004] reap cycle completed, terminated 0 instances
```

### Flap Detection

By setting the flag `--reap-flappy` to true, you will also alow reaping of flappy nodes which are detected by looking at instances of `NodeReady` events.
If a specific node's kubelet posts `NodeReady` over `--flap-count` times, the node will be considered drain-reapable.

A drain-reapable node will be cordoned & drained, and only then reaped.
By default, reaper will wait 10s post cordon and 90s post drain.
This will be followed by `--reap-throttle` seconds after the instance is terminated/reaped.

### ASG Validation

By using `--asg-validation` you are allowing reap events to occur only on the condition that the Autoscaling Group is considered stable. an Autoscaling Group will be considered stable when the number of instances and desired instances match, and also none of the instances is unhealthy.
The Autoscaling Group name is derived from the EC2 Tag of the instance which contains a `aws:autoscaling:groupName` tag which is added by the Autoscaling Group by default to all instances it spawns.

### Reaping Healthy Nodes

By setting the `--reap-old` flag, you are allowing node-reaper to reap healthy nodes. Nodes are considered old by virtue of the `--reap-old-threshold-minutes` flag, after N minutes a node will be considered old and will be drain-reaped.

Reaping healthy nodes will only happen if all the nodes in the cluster are Ready.
Master nodes are also reaped but only if there are atleast 3 healthy masters in the cluster. Also, a node will not be reaped if the `node-reaper` pod is scheduled to it - this is to avoid a situation where node-reaper drains it's own node. When nodes are old-reapable they will be drained by the oldest first.

The use of `--max-kill-nodes` can also help limit the number of nodes killed per node-reaper run, but regardless it will wait the number of seconds mentioned in `--reap-old-throttle` & `--reap-throttle` after every kill.

### Reaping "Ghost" Nodes

Ghost nodes are nodes which point to an instance-id which is invalid or already terminated. This issue has been seen in certain clusters which have a lot of churn, having low number of available IP addresses makes this more frequent, but essentially an EC2 instance is terminated for some reason, and before the node object get's removed, a new node joins with the same IP address (which assumes the same node name), this leaves the node object around, however it's `ProviderID` will reference a terminated instance ID. this can cause major problems with other controllers which rely on this value such as `alb-ingress-controller`. Enabling this feature will mean node-reaper will check that nodes `ProviderID` references a running EC2 instance, otherwise it will terminated the node. This feature is enabled by default and can be disabled by setting `--reap-ghost=false`.

### Reaping Unjoined Nodes

Unjoined nodes are nodes which fail to join the cluster and remain unjoined while taking capacity from the scaling groups. By default this feature is not enabled, but can be enabled by setting `--reap-unjoined=true`, you must also set `--reap-unjoined-threshold-minutes` which is the number of minutes passed since EC2 launch time to consider a node unjoined (we recommend setting a relatively high number here, e.g. 15), also `--reap-unjoined-tag-key` and `--reap-unjoined-tag-value` are required in order to identify the instances which failed to join, and should match an EC2 tag on the cluster nodes. when this is enabled, node-reaper will actively look at all EC2 instances with the mentioned key/value tag, and make sure they are joined in the cluster as nodes by looking at their `ProviderID`, if a matching node is not found and the EC2 instance has been up for more than the configured thershold, the instance will be terminated.

### Reaping Tainted Nodes

You can chose to mark nodes with certain taints reapable by using the `--reap-tainted` flag and providing a comma separated list of taint strings.
for example, `--reap-tainted NodeWithImpairedVolumes=true:NoSchedule,MyTaint:NoSchedule`, would mean nodes having either one of these taints will be drained & terminated. You can use the following formats for describing a taint - key=value:effect, key:effect, key.

### Example

```text
time="2019-06-13T10:00:41-07:00" level=info msg="Self Node = self-node.us-west-2.compute.internal"
time="2019-06-13T10:00:41-07:00" level=info msg="found 4 nodes, 0 pods, and 0 events"
time="2019-06-13T10:00:41-07:00" level=info msg="scanning for flappy drain-reapable nodes"
time="2019-06-13T10:00:41-07:00" level=info msg="scanning for age drain-reapable nodes"
time="2019-06-13T10:00:41-07:00" level=info msg="node node-1.us-west-2.compute.internal is drain-reapable !! State = OldAge, Diff = 43100/36000"
time="2019-06-13T10:00:41-07:00" level=info msg="node node-2.us-west-2.compute.internal is drain-reapable !! State = OldAge, Diff = 43000/36000"
time="2019-06-13T10:00:41-07:00" level=info msg="node node-3.us-west-2.compute.internal is drain-reapable !! State = OldAge, Diff = 43200/36000"
time="2019-06-13T10:00:41-07:00" level=info msg="scanning for dead nodes"
time="2019-06-13T10:00:41-07:00" level=info msg="reap cycle completed, terminated 0 instances"
time="2019-06-13T10:00:41-07:00" level=info msg="Kill order: [node-3.us-west-2.compute.internal node-1.us-west-2.compute.internal node-2.us-west-2.compute.internal]"
time="2019-06-13T10:00:41-07:00" level=info msg="draining node node-3.us-west-2.compute.internal"
...
```

### Required IAM Permissions

```text
autoscaling:TerminateInstanceInAutoScalingGroup
autoscaling:DescribeAutoScalingGroups
ec2:DescribeTags
```

### Required RBAC Permissions

```yaml
rules:
- apiGroups: [""]
  resources: ["nodes", "pods"]
  verbs: ["get", "list", "patch"]
- apiGroups: [""]
  resources: ["events"]
  verbs: ["get", "list", "create"]
- apiGroups: [""]
  resources: ["pods/eviction"]
  verbs: ["create"]
- apiGroups: ["batch"]
  resources: ["cronjobs"]
  verbs: ["get", "patch"]
- apiGroups: ["extensions", "apps"]
  resources: ["daemonsets"]
  verbs: ["get"]
```

## PDB Reaper

### What is a reapable PDB

a PDB is considered reapable if it is blocking disruptions in specific scenarios. This is perticularly useful in pre-production environments where cluster tenants use PDBs incorrectly or leave pods around in crashloop while a PDB is in place. It can also be run in production with the `--dry-run` flag in order to have a good view of which PDBs might interrupt an update.

Since pdb-reaper will not recreate the PDBs it deletes, deletion is particularly useful in cases where GitOps is used, which can re-create the PDBs at a later time.

#### Blocking PDBs due to Misconfiguration

In cases where a PDB is misconfigured, to allow 0 disruptions, it will always block node drains.
For example, if maxUnavailable is set to 0, the PDB will forever block node drains.

```yaml
apiVersion: policy/v1beta1
kind: PodDisruptionBudget
metadata:
  name: misconfigured-pdb
spec:
  maxUnavailable: 0
  selector:
    matchLabels:
      app: nginx
```

Alternatively, if minAvailable is used and the value configured matches the number of pods, the PDB will be considered reapable.

```yaml
apiVersion: policy/v1beta1
kind: PodDisruptionBudget
metadata:
  name: misconfigured-pdb
spec:
  minAvailable: 100%
  selector:
    matchLabels:
      app: nginx
```

#### Blocking PDBs due to Crashlooping Pods

When all pods are in CrashLoopBackOff, the PDB might allow zero disruption even if it is correctly configured, however it would be irrelevant to block the draining in this case since pods keep crashing. If there is atleast a single pod in the PDB's target which is CrashLoopBackOff, with more than `--crashloop-restart-count` restarts, and the PDB is blocking (allowing zero disruptions), the PDB will be considered reapable.

If `--all-crashloop` is set to false (default true), a single pod in CrashLoopBackOff with the above conditions will cause the PDB to be reapable.

```bash
NAME                    READY   STATUS             RESTARTS   AGE
nginx-5894696d4-t77mt   0/1     CrashLoopBackOff   4          65s
nginx-5894696d4-d75sx   0/1     CrashLoopBackOff   4          65s
nginx-5894696d4-hbj68   0/1     CrashLoopBackOff   4          65s
```

#### Blocking PDBs due to multiple PDBs targeting same pods

In some cases, users may create multiple PDBs which are targeting overlapping or same selectors, resulting in multiple PDBs watching the same pods. In such case, when a drain is attempted it will error out with the following message.

```bash
error: error when evicting pod "nginx-5894696d4-fprjv": This pod has more than one PodDisruptionBudget,
which the eviction subresource does not support.
```

When multiple PDBs are detected in the same namespaces with overlapping pods, both are considered reapable.

### Required RBAC Permissions

```yaml
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["list"]
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create"]
- apiGroups: ["policy"]
  resources: ["poddisruptionbudgets"]
  verbs: ["list", "delete"]
```

### Usage

```text
Usage:
  governor reap pdb [flags]

Flags:
      --all-crashloop                 Only deletes PDBs for crashlooping pods when all pods are in crashloop (default true)
      --crashloop-restart-count int   Minimum restart count to when considering pods in crashloop (default 5)
      --dry-run                       Will not actually delete PDBs
      --excluded-namespaces strings   Namespaces excluded from scanning
  -h, --help                          help for pdb
      --kubeconfig string             Absolute path to the kubeconfig file
      --local-mode                    Use cluster external auth
      --reap-crashloop                Delete PDBs which are targeting a deployment whose pods are in a crashloop
      --reap-misconfigured            Delete PDBs which are configured to not allow disruptions (default true)
      --reap-multiple                 Delete multiple PDBs which are targeting a single deployment (default true)
```

## Cordon AZ-NAT

### What does the AZ-NAT cordon tool do?

The AZ-NAT Cordon tool allows to cordon / uncordon a specific route to a NAT Gateway, for example, if there are networking issues in usw2-az1, you can use the tool to modify existing route tables to use a different NAT gateway in a healthy zone. When networking issues are resolved, you can use the tool to restore the route tables to the original state.

You can either run this as a job within the cluster, or as a script from command-line, as long as AWS credentials are provided.

### Usage

```text
Usage:
  governor cordon az-nat [flags]

Flags:
      --dry-run                 print change but don't replace route
  -h, --help                    help for az-nat
      --region string           AWS region to use
      --restore                 restores route tables to route to NAT in associated AZs
      --target-az-ids strings   comma separated list of AWS AZ IDs e.g. usw2-az1,usw2-az2
      --target-vpc-id string    vpc to target

Global Flags:
      --config string   config file (default is $HOME/.governor.yaml)
```

### Examples

```bash
# cordon AZ paths for az1 and az2 in us-west-2 (remove --dry-run flag to apply the change)
$ ./governor cordon az-nat --target-vpc-id vpc-09add63c8REDACTED --region us-west-2 --target-az-ids usw2-az1,usw2-az2

INFO[2021-09-09T12:50:39-07:00] running route cordon operation on zones: [usw2-az1 usw2-az2], dry-run: false
INFO[2021-09-09T12:50:39-07:00] replacing route-table entry in table rtb-00ab9dbf5REDACTED: 0.0.0.0/0->nat-0b3fde832REDACTED to 0.0.0.0/0->nat-0ef57ef93REDACTED
INFO[2021-09-09T12:50:40-07:00] replacing route-table entry in table rtb-0e90ce385REDACTED: 0.0.0.0/0->nat-0cc62c7ceREDACTED to 0.0.0.0/0->nat-0ef57ef93REDACTED
INFO[2021-09-09T12:50:40-07:00] execution completed, replaced 2 routes

$ ./governor cordon az-nat --target-vpc-id vpc-09add63c8REDACTED --region us-west-2 --target-az-ids usw2-az1,usw2-az2 --restore

INFO[2021-09-09T12:51:37-07:00] running route restore operation on zones: [usw2-az1 usw2-az2], dry-run: false
INFO[2021-09-09T12:51:37-07:00] replacing route-table entry in table rtb-00ab9dbf5REDACTED: 0.0.0.0/0->nat-0ef57ef93REDACTED to 0.0.0.0/0->nat-0b3fde832REDACTED
INFO[2021-09-09T12:51:37-07:00] replacing route-table entry in table rtb-0e90ce385REDACTED: 0.0.0.0/0->nat-0ef57ef93REDACTED to 0.0.0.0/0->nat-0cc62c7ceREDACTED
INFO[2021-09-09T12:51:38-07:00] execution completed, replaced 2 routes
```

