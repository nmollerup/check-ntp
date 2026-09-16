# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/en/1.0.0/)
and this project adheres to [Semantic
Versioning](http://semver.org/spec/v2.0.0.html).

## Unreleased

**Breaking change**: alert semantics and perfdata field names/meanings have
changed. See the [README migration note](README.md#migrating-from-pre-10).

### Changed
- Alerting is now based on ntpd/chrony's system offset (`SysVars.Offset`)
  instead of the offset to the system peer. Peer offset can spike 150-900ms
  during a VMware vMotion/DRS stun or an ESXi host upgrade without the actual
  system clock being wrong; alerting on system offset avoids these false
  positives while still catching real clock problems.
- Renamed perfdata fields to accurately reflect whether they are system or
  peer values: `offset` -> `sys_offset` (system) with `peer_offset` added
  (peer); `clk_jitter` and `stratum` now report system values, with the old
  peer-based values moved to `peer_jitter` and `peer_stratum`.
- Migrated off the archived `github.com/facebookincubator/ntp` module to its
  actively maintained fork, `github.com/facebook/time`.

### Added
- CRITICAL guard for an unsynchronized ntpd/chrony, checked before the offset
  comparison: leap indicator == 3, no system peer selected, or stratum >= 16.
- `root_distance` metric (`RootDelay/2 + RootDisp`), with optional
  `--root-distance-warning`/`--root-distance-critical` thresholds (ms). When
  left unset (0), the metric is still emitted but never alerts.
- `--legacy-perfdata` flag to additionally emit the old `offset`/
  `clk_jitter`/`stratum` field names with their old (peer-based) meaning, to
  ease migration of existing InfluxDB series and Grafana panels.

## [0.1.0] - 2021-01-02

### Added
- Initial release
