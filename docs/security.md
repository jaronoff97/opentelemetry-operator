# Security Guidance

For users with Gatekeeper installed, it's recommended to add policies to limit the Operator's ClusterRole.
Below is an example of a policy that allows you to restrict 

```yaml
apiVersion: templates.gatekeeper.sh/v1beta1
kind: ConstraintTemplate
metadata:
  name: k8srequiredserviceaccount
spec:
  crd:
    spec:
      names:
        kind: K8sRequiredServiceAccount
      validation:
        openAPIV3Schema:
          properties:
            allowedServiceAccounts:
              type: array
              items:
                type: object
                properties:
                  namespace:
                    type: string
                  serviceAccount:
                    type: string
  targets:
    - target: admission.k8s.gatekeeper.sh
      rego: |
        package k8srequiredserviceaccount

        violation[{"msg": msg}] {
          input.review.kind.kind == "Pod"
          sa := input.review.object.spec.serviceAccountName
          ns := input.review.object.metadata.namespace
          allowed_sas := {{"namespace": ns, "serviceAccount": sa} |
            entry := input.parameters.allowedServiceAccounts[_]
            entry.namespace == ns
            entry.serviceAccount == sa
          }
          not allowed_sas
          msg := sprintf("Service account %v in namespace %v is not allowed.", [sa, ns])
        }
```

And an example of the constraint:
```yaml
apiVersion: constraints.gatekeeper.sh/v1beta1
kind: K8sRequiredServiceAccount
metadata:
  name: service-account-constraint
spec:
  match:
    kinds:
      - apiGroups: [""]
        kinds: ["Pod"]
  parameters:
    allowedServiceAccounts:
      - namespace: "opentelemetry-operator-system"
        serviceAccount: "opentelemetry-operator"
      ...
```

