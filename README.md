# cyclist 🚴

µService™ for managing AWS auto-scaling groups and their lifecycle events.

By default, ASGs will forcefully shut your instance down when scaling in. In
order to prevent that, the lifecycle must be actively handled by responding
to events via lifecycle hooks.

This is mostly for running instances of
[worker](https://github.com/travis-ci/worker) on AWS.

On **scale-in** we do not want to cancel currently running jobs. Instead, we
want to stop accepting new work, finish the existing jobs, and then shut down.

On **scale-out** we want to do some initial set-up (downloading docker images)
before taking on work.

For scaling in, cyclist will receive a termination request via SNS. It will
notify the instance that is to be retired to shut down gracefully (all workers
poll for this condition). The instance finishes the jobs and notifies cyclist
that it is ready to shut down. Cyclist then terminates the instance.

## Install

``` bash
make
```

## Develop

``` bash
make dev-server
```
