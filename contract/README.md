# The helpdesk contract

What the operator organization needs on the platform to run the helpdesk: its
Thing Type and NATS role. It is a `stone` template, applied once, as an owner
or admin of the operator organization:

```sh
stone org switch <operator-org>
stone apply --template contract/ --dry-run
stone apply --template contract/
```

Applying it again updates the organization to this version and changes
nothing that already matches. See `stone apply --help` (TEMPLATES).

| Thing Type | Process | NATS role |
|---|---|---|
| `helpdesk` | the helpdesk server | `helpdesk` |

A managed organization needs nothing from this folder. Its tickets reach the
helpdesk through the `helpdesk.>` export the platform creates for it.

Keep the role in step with `internal/subjects` and the stream names in
`config`.
