---
title: Destructive demo (acknowledgement)
slug: destructive-demo
order: 2
description: A fixture for the acknowledgement pattern — opening this runbook gates the page behind a modal until you accept it.
acknowledge: "This drops and rebuilds the replica. Take it out of the load balancer before you start — traffic on the primary keeps coming."
symptoms:
  - acknowledgement
  - destructive
---

This runbook exists to show the acknowledgement gate. If you can read this without a dialog first, something is wrong.

## Confirm the target

Check you are pointing at the replica you meant to rebuild.

```bash [Check the target]
mysql -h db-replica -e "SELECT @@hostname, @@read_only;"
```

## Rebuild the replica

The destructive step, reached only after the acknowledgement.

```bash [Rebuild]
mysql -h db-replica -e "STOP REPLICA; RESET REPLICA ALL;"
```
