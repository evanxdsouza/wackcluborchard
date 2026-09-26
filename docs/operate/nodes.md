# Nodes and pools

**Instance admin → Nodes & pools** shows every node's readiness, roles, kubelet,
and requested CPU and memory against *allocatable* (smaller than capacity, since
the system takes a share).

A **pool** is a set of nodes with a purpose (builds, a big-memory box, hardware
someone paid for) assigned to organizations. Orchard labels the nodes
`orchard.dev/pool=<name>`, optionally taints them `NoSchedule`, and gives the
organizations' workloads the matching node selector and toleration. Because it
is ordinary Kubernetes scheduling, `kubectl describe node` explains placement.

When a pod will not schedule, the app says why in plain English: not enough
allocatable CPU or memory, an untolerated taint, or a single-writer volume
attached elsewhere.
