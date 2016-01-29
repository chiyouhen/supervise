# supervise
Service supervisor. Keep process running, auto pull it up.

# Compiling
[kesselborn/go-getopt](https://github.com/kesselborn/go-getopt) is required. Clone the repository in `github.com/kesselborn/go-getopt` relative to src path.
```
go install supervise.go
```

# Command Line Argument
```
supervise -d [run_dir] -c "cmd" <-D>
```
- `run_dir` is a dir store some file used for supervise.
- `cmd` is a string, supervise just run `/bin/bash -c cmd`, so test the cmd with `bash -c`.
- `-D` to make supervise as daemon.

# Control
After supervise started, a pipe `control` established in `run_dir`. This pipe can recvieve following character:
- `x` stop service and quit supervise.
- `d` stop service, keep supervise running.
- `u` after service stoped by `d`, `u` start the service again. Note: if supervise is not running, `echo u > control` will cause blocking script.
- `k` send KILL to service.
