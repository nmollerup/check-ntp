[![Sensu Bonsai Asset](https://img.shields.io/badge/Bonsai-Download%20Me-brightgreen.svg?colorB=89C967&logo=sensu)](https://bonsai.sensu.io/assets/nmollerup/check-ntp)
[![Go Test](https://github.com/nmollerup/check-ntp/actions/workflows/test.yml/badge.svg)](https://github.com/nmollerup/check-ntp/actions/workflows/test.yml)
![goreleaser](https://github.com/nmollerup/check-ntp/workflows/goreleaser/badge.svg)

# Sensu NTP offset check for Linux (ntpd or chrony)

## Table of Contents

- [Overview](#overview)
- [Usage examples](#usage-examples)
- [Perfdata](#perfdata)
- [Migrating from pre-1.0](#migrating-from-pre-10)
- [Configuration](#configuration)
  - [Asset registration](#asset-registration)
  - [Check definition](#check-definition)
- [Installation from source](#installation-from-source)
- [Contributing](#contributing)

## Overview

The Sensu NTP Check is a [Sensu Check][1] that provides alerting on NTP offset
and a set of NTP metrics. Metrics are provided in [nagios_perfdata][5] format.
This check supports both ntpd and chrony NTP services running under Linux.

Alerting is based on ntpd/chrony's **system offset** (the corrected clock
offset applied to the local clock), not the offset to any individual peer.
Peer offset can spike transiently (for example, when a VM is stunned by a
vMotion/DRS migration or an ESXi host upgrade, which delays packet
timestamps) without the system clock itself being wrong. The check also
verifies that ntpd/chrony reports itself as synchronized (leap indicator, sys
peer selection, and stratum) before evaluating the offset thresholds.

## Usage examples

```
Check NTP offset and provide metrics

Usage:
  check-ntp [flags]
  check-ntp [command]

Available Commands:
  help        Help about any command
  version     Print the version number of this plugin

Flags:
  -c, --critical float                 Critical threshold for system offset in ms (default 100)
  -w, --warning float                  Warning threshold for system offset in ms (default 10)
      --root-distance-critical float   Critical threshold for root distance in ms (0 disables alerting, metric is still emitted)
      --root-distance-warning float    Warning threshold for root distance in ms (0 disables alerting, metric is still emitted)
      --legacy-perfdata                Additionally emit old perfdata field names (offset, clk_jitter, stratum) with their old (peer-based) meaning
  -h, --help                           help for check-ntp

Use "check-ntp [command] --help" for more information about a command.
```

## Perfdata

All metrics are reported in [nagios_perfdata][5] format:
`check-ntp STATE: message | k=v, k=v, ...`.

| Metric          | Value type | Unit | Description                                                        |
|-----------------|------------|------|----------------------------------------------------------------------|
| `sys_offset`    | System     | ms   | Corrected system clock offset. This drives the -w/-c alert.          |
| `stratum`       | System     | -    | ntpd/chrony's own reported stratum.                                  |
| `root_distance` | System     | ms   | `RootDelay/2 + RootDisp`; distance from the reference clock.         |
| `sys_jitter`    | System     | ms   | System clock jitter.                                                 |
| `clk_jitter`    | System     | ms   | Clock discipline jitter.                                             |
| `clk_wander`    | System     | PPM  | Clock frequency wander.                                              |
| `frequency`     | System     | PPM  | Clock frequency correction.                                          |
| `tc`             | System     | -    | Clock discipline time constant.                                      |
| `mintc`          | System     | -    | Minimum clock discipline time constant.                              |
| `peer_offset`   | Peer       | ms   | Offset to the selected system peer (or an average of good peers).    |
| `peer_jitter`   | Peer       | ms   | Jitter of the selected system peer.                                  |
| `peer_stratum`  | Peer       | -    | Stratum reported by the selected system peer.                        |

`--root-distance-warning`/`--root-distance-critical` are optional; when left
at their default of `0`, `root_distance` is still reported as a metric but
never triggers an alert.

With `--legacy-perfdata`, three additional keys are appended using the
**pre-migration names and meanings** (peer-based, not system-based):
`offset` (= `peer_offset`), `clk_jitter` (= `peer_jitter`), `stratum` (=
`peer_stratum`). Note that in this mode the perfdata line contains two
`clk_jitter=` and two `stratum=` entries with different values — the first
occurrence is always the new, system-based metric; the appended, legacy one
is peer-based. Use `--legacy-perfdata` only as a temporary migration aid.

## Migrating from pre-1.0

Versions before 1.0 alerted on peer offset and named perfdata fields
`offset`, `clk_jitter`, and `stratum` using **peer** values. As of 1.0:

- Alerting is based on `sys_offset` (system offset), not peer offset.
- `offset` has been renamed to `sys_offset` (system-based) and `peer_offset`
  (peer-based) is a new, separate metric.
- `clk_jitter` and `stratum` now report **system** values instead of peer
  values; the old peer-based values are available under `peer_jitter` and
  `peer_stratum`.

If you have existing InfluxDB series or Grafana panels built on the old
`offset`/`clk_jitter`/`stratum` measurements, either repoint them at the new
field names, or run the check with `--legacy-perfdata` during the transition
to keep emitting the old names with their old (peer-based) values alongside
the new ones.

## Configuration

### Asset registration

[Sensu Assets][2] are the best way to make use of this plugin. If you're not
using an asset, please consider doing so! If you're using sensuctl 5.13 with
Sensu Backend 5.13 or later, you can use the following command to add the asset:

```
sensuctl asset add nmollerup/check-ntp
```

If you're using an earlier version of sensuctl, you can find the asset on the [Bonsai Asset Index][3].

### Check definition

```yml
---
type: CheckConfig
api_version: core/v2
metadata:
  name: check-ntp
  namespace: default
spec:
  command: >-
    check-ntp
    --critical 250
    --warning 150
  output_metric_format: nagios_perfdata
  output_metric_handlers:
    - influxdb
  subscriptions:
  - system
  runtime_assets:
  - nmollerup/check-ntp
```

## Installation from source

The preferred way of installing and deploying this plugin is to use it as an
Asset. If you would like to compile and install the plugin from source or
contribute to it, download the latest version or create an executable from this
source.

From the local path of the check-ntp repository:

```
go build
```

## Contributing

For more information about contributing to this plugin, see [Contributing][4].

[1]: https://docs.sensu.io/sensu-go/latest/reference/checks/
[2]: https://docs.sensu.io/sensu-go/latest/reference/assets/
[3]: https://bonsai.sensu.io/assets/nixwiz/check-ntp
[4]: https://github.com/sensu/sensu-go/blob/master/CONTRIBUTING.md
[5]: https://docs.sensu.io/sensu-go/latest/observability-pipeline/observe-schedule/collect-metrics-with-checks/#supported-output-metric-formats
