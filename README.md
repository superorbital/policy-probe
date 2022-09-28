# kubectl-probe (name to be bikeshed)

## Usage
```
kubectl probe --config probe.yaml
```

--- 
Assert that probe `foo` running in the `a` namespace can talk to to `test-service` running in the `b` namespace.
This will:
- Create a probe pod with the specified labels
- Assert that it is unable to reach any port on the `test-service` listed on the service
```yaml
kind: Probe
apiVersion: probe.superorbital.io/v1alpha1
spec:
  expect: Fail
  from:
    probe:
      namespace: a
      labels:
        role: api
  to:
    service:
      namespace: b
      name: test-service
      protocol: udp
```

---
Assert that Deployment `foo` running in the `a` namespace can talk to to `test-service:5000`.
This will:
- If supported start an ephemeral probe container in one of the deployments pods
- If not supported or the deployment has no pods, create a probe pod with the same labels[^1]
```yaml
kind: Probe
apiVersion: probe.superorbital.io/v1alpha1
spec:
  expect: Pass
  from:
    deployment:
      namespace: a
      name: foo
  to:
    server:
      port: 5000
      host: test-service
      protocol: tcp
```

## Configuration

`expect`: `"Pass" | "Fail"` whether the probe is expected to succeed or fail

`from`:
- `Deployment`: if it has any running pods we'll just pick one, otherwise we can clone its PodSpec
- `Pod`: an existing pod we will either add an ephemeral pod to or "clone" 
- `Probe`: a pod we will spin up that will send traffic to the `to` (maybe via `nc` or something similar)
 specify an existing Deployment or Pod, or specify a namespace and optional labels to create a Probe pod from

`to`: 
- `Deployment`
- `Pod`
- `Service`
- `Probe`: a pod we'll spin up to receive traffic
- `Server`: an arbitrary host/server address


## Open questions
- When using UDP we will need to have a receiver service running to confirm that the packets are actually going anywhere. With TCP we can just rely on the handshake succeeding to be confident enough for now that its network policy isn't getting in the way.

[^1]: When ephemeral pods aren't available as a feature we could use the [ksniff](https://github.com/eldadru/ksniff) approach and create a new container on the same network namespace as the targeted pod, or we can just create a new pod with the same labels. The former seems more robust.

