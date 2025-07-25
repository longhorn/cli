## longhornctl install preflight stop

Stop Longhorn preflight installer

### Synopsis

This command terminates the preflight installer.

```
longhornctl install preflight stop [flags]
```

### Examples

```
$ longhornctl install preflight stop
INFO[2024-07-16T17:21:32+08:00] Stopping preflight installer
INFO[2024-07-16T17:21:32+08:00] Successfully stopped preflight installer
```

### Options

```
  -h, --help                      help for stop
      --image string              Image containing longhornctl-local (default "longhornio/longhorn-cli:v1.10.0-dev")
      --kube-config string        Kubernetes config (kubeconfig) path
  -l, --log-level string          Log level (default "info")
      --node-selector string      Comma-separated list of key=value pairs to match against node labels, selecting the nodes the DaemonSet will run on (e.g. env=prod,zone=us-west).
      --operating-system string   Specify the operating system ("", cos). Leave this empty to use the package manager for installation.
```

### SEE ALSO

* [longhornctl install preflight](longhornctl_install_preflight.md)	 - Install Longhorn preflight

###### Auto generated by spf13/cobra on 8-Jul-2025
