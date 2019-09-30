## Pod Reaper

### What is a reapable pod

A pod is determined to be 'reapable' by reaper in the following conditions:

- It is terminating (by virtue of `Metadata.DeletionTimestamp != IsZero()`)

- The difference between the [<sup>1</sup>](#1)`adjusted deletion timestamp` and time of reaping is greater than the configurable thresholds (`--reap-after` flag)

- By default, reaper will use `Soft Reaping` which means it will not reap pods that still have containers running, this can be turned off by setting `--soft-reap=false`

<a class="anchor" id="1">1. </a>Adjusted deletion timestamp is calculated by `sum(Metadata.DeletionTimestamp - Metadata.DeletionGracePeriodSeconds - Spec.TerminationGracePeriodSeconds)`

### Usage

```text
Usage:
  governor reap pod [flags]

Flags:
      --dry-run             Will not terminate pods
  -h, --help                help for pod
      --kubeconfig string   Absolute path to the kubeconfig file
      --local-mode          Use cluster external auth
      --reap-after float    Reaping threshold in minutes (default 10)
      --soft-reap           Will not terminate pods with running containers (default true)
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

```text
INFO[2019-01-14T23:29:51-08:00] starting cluster external auth
INFO[2019-01-14T23:29:51-08:00] kubeconfig: ~/.kube/config
INFO[2019-01-14T23:29:51-08:00] target: https://api-eytan-test-cluster-k8-ph1nmb-1654871915.us-west-2.elb.amazonaws.com
INFO[2019-01-14T23:29:51-08:00] starting scan cycle
INFO[2019-01-14T23:29:52-08:00] found 69 pods
INFO[2019-01-14T23:29:52-08:00] kube-system/pod-1 is being deleted
INFO[2019-01-14T23:29:52-08:00] kube-system/pod-2 is being deleted
INFO[2019-01-14T23:29:52-08:00] kube-system/pod-3 is being deleted
INFO[2019-01-14T23:29:52-08:00] reap time = 2019-01-15 07:29:52.1688 +0000 UTC
INFO[2019-01-14T23:29:52-08:00] reap target threshold = 6m
INFO[2019-01-14T23:29:52-08:00] kube-system/pod-1 total grace period = 3630s
INFO[2019-01-14T23:29:52-08:00] kube-system/pod-1 has been terminating since 2019-01-15 07:19:37 +0000 UTC, diff: 10.25/6
INFO[2019-01-14T23:29:52-08:00] kube-system/pod-1 is reapable !!
INFO[2019-01-14T23:29:52-08:00] kube-system/pod-2 total grace period = 3630s
INFO[2019-01-14T23:29:52-08:00] kube-system/pod-2 has been terminating since 2019-01-15 07:23:17 +0000 UTC, diff: 8.59/6
INFO[2019-01-14T23:29:52-08:00] kube-system/pod-2 is not reapable - running containers detected
INFO[2019-01-14T23:29:52-08:00] start reap cycle
INFO[2019-01-14T23:29:52-08:00] kube-system/pod-3 total grace period = 3630s
INFO[2019-01-14T23:29:52-08:00] kube-system/pod-3 has been terminating since 2019-01-15 07:27:17 +0000 UTC, diff: 2.59/6
INFO[2019-01-14T23:29:52-08:00] kube-system/pod-3 is not reapable - did not meet reapAfter threshold
INFO[2019-01-14T23:29:52-08:00] start reap cycle
INFO[2019-01-14T23:29:52-08:00] reaping kube-system/pod-1
INFO[2019-01-14T23:29:52-08:00] pod dump: {"metadata":{"name":"pod-1","namespace":"kube-system","selfLink":"/api/v1/namespaces/kube-system/pods/pod-1","uid":"f9554780-1895-11e9-9918-0ace961425a6","resourceVersion":"1349217","creationTimestamp":"2019-01-15T07:20:02Z","deletionTimestamp":"2019-01-15T08:20:07Z","deletionGracePeriodSeconds":3600,"labels":{"app":"pod-1"}},"spec":{"volumes":[{"name":"default-token-vgrjj","secret":{"secretName":"default-token-vgrjj","defaultMode":420}}],"containers":[{"name":"busybox","image":"busybox","command":["sleep","3600"],"resources":{},"volumeMounts":[{"name":"default-token-vgrjj","readOnly":true,"mountPath":"/var/run/secrets/kubernetes.io/serviceaccount"}],"terminationMessagePath":"/dev/termination-log","terminationMessagePolicy":"File","imagePullPolicy":"IfNotPresent"}],"restartPolicy":"Always","terminationGracePeriodSeconds":30,"dnsPolicy":"ClusterFirst","serviceAccountName":"default","serviceAccount":"default","nodeName":"ip-10-105-234-237.us-west-2.compute.internal","securityContext":{},"schedulerName":"default-scheduler","tolerations":[{"key":"node.kubernetes.io/not-ready","operator":"Exists","effect":"NoExecute","tolerationSeconds":300},{"key":"node.kubernetes.io/unreachable","operator":"Exists","effect":"NoExecute","tolerationSeconds":300}],"priority":0},"status":{"phase":"Running","conditions":[{"type":"Initialized","status":"True","lastProbeTime":null,"lastTransitionTime":"2019-01-15T07:20:02Z"},{"type":"Ready","status":"True","lastProbeTime":null,"lastTransitionTime":"2019-01-15T07:20:03Z"},{"type":"ContainersReady","status":"True","lastProbeTime":null,"lastTransitionTime":"2019-01-15T07:20:03Z"},{"type":"PodScheduled","status":"True","lastProbeTime":null,"lastTransitionTime":"2019-01-15T07:20:02Z"}],"hostIP":"10.105.234.237","podIP":"100.98.101.92","startTime":"2019-01-15T07:20:02Z","containerStatuses":[{"name":"busybox","state":{"running":{"startedAt":"2019-01-15T07:20:03Z"}},"lastState":{},"ready":true,"restartCount":0,"image":"busybox:latest","imageID":"docker-pullable://busybox@sha256:7964ad52e396a6e045c39b5a44438424ac52e12e4d5a25d94895f2058cb863a0","containerID":"docker://13e47483f2f80a529a12a473b2fa0e429793edceaee24b43da3616d7cb09281b"}],"qosClass":"BestEffort"}}
WARN[2019-01-14T23:29:52-08:00] dry-run is on, pod will not be reaped
```

### Required RBAC Permissions

```yaml
rules:
- apiGroups: [""]
  resources: ["pods", ]
  verbs: ["get", "delete", "list"]
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
