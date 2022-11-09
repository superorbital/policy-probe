# kubectl-probe (name to be bikeshed)

## Usage

```bash
kubectl probe -f testsuite.yaml
```

---
Assert that deployment `web` running in the `a` namespace can't talk to `api.prox:80`, but that `web` in the `b` namespace can.
This will:

- Attach ephemeral probes to pods in both deployments
- Assert that the `a` pod is unable to reach `api.prox:80`, but the `b` pod is

```yaml
apiVersion: networking.superorbital.io/v1beta1
kind: TestSuite
metadata:
  name: namespaces
spec:
  testCases:
    - description: Pod in namespace a **cannot** reach api.prod
      expect: Fail
      from:
        deployment:
          name: web
          namespace: a
      to:
        address: api.prox:80
    - description: Pod in namespace b **can** reach api.prod
      expect: Pass
      from:
        deployment:
          name: web
          namespace: b
      to:
        address: api.prod:80

```

## Configuration

`expect`: `"Pass" | "Fail"` whether the probe is expected to succeed or fail

`from`:

- `deployment`: a probe will be attached to one of its pods chosen randomly

 specify an existing Deployment or Pod, or specify a namespace and optional labels to create a Probe pod from

`to`:

- `address`: a hostname/port combination, for example `example.com:80`