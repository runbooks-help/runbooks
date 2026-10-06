---
title: Replication Lag (MTS Deadlock)
slug: replication-lag
order: 3
description: "Replica SQL workers stalled on handler commit: the replica is up but lag grows. Break the deadlock without a full reseed."
common: true
symptoms:
  - Replication lag
  - replica behind, Seconds_Behind_Source growing
  - replica SQL workers stuck, IO thread healthy
  - SHOW PROCESSLIST waiting for handler commit
  - lag not going down
vars:
  - id: replication-processlist-id
    label: Blocked worker's processlist Id
    var: PROCESSLIST_ID
    placeholder: "e.g. 4123"
    hint: |-
      From the Id column of the SHOW PROCESSLIST output; pick a worker stuck in
      "waiting for handler commit".
notice: "Demo content: a realistic procedure, but nothing here touches a real host."
---

A replica using multi-threaded replication (MTS) can deadlock its SQL workers on a
handler commit. The replica is up, the IO thread is healthy, and lag grows without
bound. It does not clear on its own.

## Diagnosis

### Identify the MTS deadlock pattern

A healthy MTS replica shows most workers in `Waiting for an event from Coordinator`.
A deadlocked one shows a specific constellation of blocked threads.

```sql [Check processlist for the deadlock pattern]
SHOW PROCESSLIST;
```

| State | Meaning |
|-------|---------|
| `Waiting for source to send event` | IO thread, healthy, reading the binlog |
| `Waiting for dependent transaction to commit` | Worker blocked on another worker's commit |
| `waiting for handler commit` | Worker stuck on a binlog flush or handler commit lock |
| `Waiting for an event from Coordinator` | Starved workers, nothing dispatched |

The signature: one or more workers in `waiting for handler commit`, another in
`Waiting for dependent transaction to commit`, and the rest starved in `Waiting for
an event from Coordinator`. The IO thread stays healthy; the problem is all on the
SQL side.

### Confirm the lag is not just a big transaction

```bash [Check Seconds_Behind_Source]
mysql -e "SHOW REPLICA STATUS\G" | grep -E "Replica_IO_Running|Replica_SQL_Running|Seconds_Behind"
```

`Seconds_Behind_Source` grows steadily and the SQL thread is running but making no
progress. A single huge transaction looks similar, so check the engine status before
killing anything.

```sql [Look for ACTIVE (PREPARED) transactions]
SHOW ENGINE INNODB STATUS\G
```

If the `TRANSACTIONS` section shows one transaction holding the commit lock, this is
not the MTS deadlock; let it finish and lag will drain.

> [!branch] Is there a single ACTIVE (PREPARED) transaction?
> - **Yes** — it is a large transaction, not a deadlock. Wait for it to commit; go to the rollback note.
> - **No** — it is the MTS deadlock. Continue below.

## Recovery

### Free the blocked worker

Killing the worker stuck in `waiting for handler commit` releases the lock; the
coordinator re-dispatches and the workers drain.

```sql [Release the blocked worker]
KILL {{PROCESSLIST_ID}};
```

```sql [Watch the workers drain]
SHOW PROCESSLIST;
```

Within a few seconds every worker should be back to `Waiting for an event from
Coordinator` or `Waiting for dependent transaction to commit`, and
`Seconds_Behind_Source` should fall.

### Confirm the replica caught up

```bash [Confirm lag is falling]
mysql -e "SHOW REPLICA STATUS\G" | grep Seconds_Behind
```

If the value is still climbing after the kill, repeat the diagnosis: a second worker
may be stuck behind the first.

---rollback

If killing the worker does not clear the deadlock, or lag is still climbing after two
attempts, stop the SQL thread and let it restart cleanly. This is disruptive: the
replica serves stale reads until it catches up.

```sql [Stop and restart the SQL thread]
STOP REPLICA SQL_THREAD;
START REPLICA SQL_THREAD;
```

If that still fails, the replica needs a rebuild from the primary. Escalate before
rebuilding: a reseed is a full copy and takes the replica out of the pool.
