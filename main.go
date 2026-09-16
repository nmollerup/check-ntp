package main

import (
	"fmt"
	"math"
	"strings"

	"github.com/facebook/time/cmd/ntpcheck/checker"
	corev2 "github.com/sensu/sensu-go/api/core/v2"
	"github.com/sensu/sensu-plugin-sdk/sensu"
)

const (
	// leapAlarm is the NTP leap indicator value meaning "not synchronized".
	leapAlarm = 3
	// unsyncStratum is the stratum value (or above) NTP uses to mean "not synchronized".
	unsyncStratum = 16
)

// Config represents the check plugin config.
type Config struct {
	sensu.PluginConfig
	Warning              float64
	Critical             float64
	RootDistanceWarning  float64
	RootDistanceCritical float64
	LegacyPerfdata       bool
}

var (
	plugin = Config{
		PluginConfig: sensu.PluginConfig{
			Name:     "check-ntp",
			Short:    "Check NTP offset and provide metrics",
			Keyspace: "sensu.io/plugins/check-ntp/config",
		},
	}

	options = []sensu.ConfigOption{
		&sensu.PluginConfigOption[float64]{
			Path:      "critical",
			Argument:  "critical",
			Shorthand: "c",
			Default:   float64(100),
			Usage:     "Critical threshold for system offset in ms",
			Value:     &plugin.Critical,
		},
		&sensu.PluginConfigOption[float64]{
			Path:      "warning",
			Argument:  "warning",
			Shorthand: "w",
			Default:   float64(10),
			Usage:     "Warning threshold for system offset in ms",
			Value:     &plugin.Warning,
		},
		&sensu.PluginConfigOption[float64]{
			Path:     "root-distance-critical",
			Argument: "root-distance-critical",
			Default:  float64(0),
			Usage:    "Critical threshold for root distance in ms (0 disables alerting, metric is still emitted)",
			Value:    &plugin.RootDistanceCritical,
		},
		&sensu.PluginConfigOption[float64]{
			Path:     "root-distance-warning",
			Argument: "root-distance-warning",
			Default:  float64(0),
			Usage:    "Warning threshold for root distance in ms (0 disables alerting, metric is still emitted)",
			Value:    &plugin.RootDistanceWarning,
		},
		&sensu.PluginConfigOption[bool]{
			Path:     "legacy-perfdata",
			Argument: "legacy-perfdata",
			Default:  false,
			Usage:    "Additionally emit old perfdata field names (offset, clk_jitter, stratum) with their old (peer-based) meaning",
			Value:    &plugin.LegacyPerfdata,
		},
	}
)

func main() {
	check := sensu.NewCheck(&plugin.PluginConfig, options, checkArgs, executeCheck, false)
	check.Execute()
}

func checkArgs(event *corev2.Event) (int, error) {
	if plugin.Critical == 0 {
		return sensu.CheckStateWarning, fmt.Errorf("--critical is required")
	}
	if plugin.Warning == 0 {
		return sensu.CheckStateWarning, fmt.Errorf("--warning is required")
	}
	if plugin.Warning > plugin.Critical {
		return sensu.CheckStateWarning, fmt.Errorf("--warning cannot be greater than --critical")
	}
	if plugin.RootDistanceWarning != 0 && plugin.RootDistanceCritical != 0 && plugin.RootDistanceWarning > plugin.RootDistanceCritical {
		return sensu.CheckStateWarning, fmt.Errorf("--root-distance-warning cannot be greater than --root-distance-critical")
	}
	return sensu.CheckStateOK, nil
}

func executeCheck(event *corev2.Event) (int, error) {
	result, err := checker.RunCheck("")
	if err != nil {
		fmt.Printf("%s CRITICAL: failed to run check, error: %v\n", plugin.Name, err)
		return sensu.CheckStateCritical, nil
	}
	if result.SysVars == nil {
		fmt.Printf("%s CRITICAL: failed to extract NTP statistics, error: no system variables\n", plugin.Name)
		return sensu.CheckStateCritical, nil
	}

	stats, err := checker.NewNTPStats(result)
	if err != nil {
		// If ntpd/chrony isn't synchronized, evaluate() will already report a
		// specific CRITICAL for it below, without needing peer stats. Only
		// treat this as its own fatal error when synchronization looks fine
		// but we still couldn't derive peer stats for some other reason.
		_, sysPeerErr := result.FindSysPeer()
		unsynchronized := result.SysVars.Leap == leapAlarm || result.SysVars.Stratum >= unsyncStratum || sysPeerErr != nil
		if !unsynchronized {
			fmt.Printf("%s CRITICAL: failed to extract NTP statistics, error: %v\n", plugin.Name, err)
			return sensu.CheckStateCritical, nil
		}
		stats = nil
	}

	state, output := evaluate(result, stats, plugin)
	fmt.Println(output)
	return state, nil
}

// evaluate is the pure decision function: given an NTP check result (and its
// derived stats, which may be nil if they couldn't be computed), it decides
// the Sensu check state and formats the nagios_perfdata output line.
func evaluate(result *checker.NTPCheckResult, stats *checker.NTPStats, cfg Config) (int, string) {
	rootDistance := result.SysVars.RootDelay/2 + result.SysVars.RootDisp
	perfData := buildPerfData(result, stats, rootDistance, cfg.LegacyPerfdata)

	if result.SysVars.Leap == leapAlarm {
		return sensu.CheckStateCritical, fmt.Sprintf("%s CRITICAL: ntpd leap indicator reports unsynchronized (leap=%d) | %s", cfg.Name, result.SysVars.Leap, perfData)
	}
	if result.SysVars.Stratum >= unsyncStratum {
		return sensu.CheckStateCritical, fmt.Sprintf("%s CRITICAL: stratum %d is unsynchronized | %s", cfg.Name, result.SysVars.Stratum, perfData)
	}
	if _, err := result.FindSysPeer(); err != nil {
		return sensu.CheckStateCritical, fmt.Sprintf("%s CRITICAL: no system peer selected | %s", cfg.Name, perfData)
	}

	offsetState, offsetMsg := evaluateOffset(result.SysVars.Offset, cfg)
	rootDistanceState, rootDistanceMsg := evaluateRootDistance(rootDistance, cfg)

	state := offsetState
	if rootDistanceState > state {
		state = rootDistanceState
	}

	messages := []string{offsetMsg}
	if rootDistanceMsg != "" {
		messages = append(messages, rootDistanceMsg)
	}

	return state, fmt.Sprintf("%s %s: %s | %s", cfg.Name, stateLabel(state), strings.Join(messages, "; "), perfData)
}

// evaluateOffset compares the system offset against the warning/critical
// thresholds, using the absolute value so negative offsets are handled too.
func evaluateOffset(offset float64, cfg Config) (int, string) {
	abs := math.Abs(offset)
	switch {
	case abs > cfg.Critical:
		return sensu.CheckStateCritical, fmt.Sprintf("system offset %.3fms exceeds critical threshold %.3fms", offset, cfg.Critical)
	case abs > cfg.Warning:
		return sensu.CheckStateWarning, fmt.Sprintf("system offset %.3fms exceeds warning threshold %.3fms", offset, cfg.Warning)
	default:
		return sensu.CheckStateOK, fmt.Sprintf("system offset %.3fms within thresholds", offset)
	}
}

// evaluateRootDistance compares root distance (RootDelay/2 + RootDisp)
// against its thresholds. A threshold of 0 means "unset": it never alerts,
// and no message is contributed (the metric is still emitted via perfdata).
func evaluateRootDistance(rootDistance float64, cfg Config) (int, string) {
	switch {
	case cfg.RootDistanceCritical != 0 && rootDistance > cfg.RootDistanceCritical:
		return sensu.CheckStateCritical, fmt.Sprintf("root distance %.3fms exceeds critical threshold %.3fms", rootDistance, cfg.RootDistanceCritical)
	case cfg.RootDistanceWarning != 0 && rootDistance > cfg.RootDistanceWarning:
		return sensu.CheckStateWarning, fmt.Sprintf("root distance %.3fms exceeds warning threshold %.3fms", rootDistance, cfg.RootDistanceWarning)
	default:
		return sensu.CheckStateOK, ""
	}
}

func stateLabel(state int) string {
	switch state {
	case sensu.CheckStateCritical:
		return "CRITICAL"
	case sensu.CheckStateWarning:
		return "WARNING"
	default:
		return "OK"
	}
}

// buildPerfData formats the nagios_perfdata metrics line. stats may be nil
// (e.g. when ntpd/chrony has no usable peers at all), in which case the
// peer_* metrics are reported as zero.
func buildPerfData(result *checker.NTPCheckResult, stats *checker.NTPStats, rootDistance float64, legacy bool) string {
	var peerOffset, peerJitter float64
	var peerStratum int
	if stats != nil {
		peerOffset = stats.PeerOffset
		peerJitter = stats.PeerJitter
		peerStratum = stats.PeerStratum
	}

	parts := []string{
		fmt.Sprintf("sys_offset=%f", result.SysVars.Offset),
		fmt.Sprintf("peer_offset=%f", peerOffset),
		fmt.Sprintf("peer_jitter=%f", peerJitter),
		fmt.Sprintf("peer_stratum=%d", peerStratum),
		fmt.Sprintf("stratum=%d", result.SysVars.Stratum),
		fmt.Sprintf("root_distance=%f", rootDistance),
		fmt.Sprintf("sys_jitter=%f", result.SysVars.SysJitter),
		fmt.Sprintf("clk_jitter=%f", result.SysVars.ClkJitter),
		fmt.Sprintf("clk_wander=%f", result.SysVars.ClkWander),
		fmt.Sprintf("frequency=%f", result.SysVars.Frequency),
		fmt.Sprintf("tc=%d", result.SysVars.TC),
		fmt.Sprintf("mintc=%d", result.SysVars.MinTC),
	}

	if legacy {
		// Old names, old (peer-based) meaning - kept for InfluxDB/Grafana
		// series built against the pre-migration field names. These
		// intentionally duplicate the "clk_jitter" and "stratum" keys above
		// with different (peer-based) values.
		parts = append(parts,
			fmt.Sprintf("offset=%f", peerOffset),
			fmt.Sprintf("clk_jitter=%f", peerJitter),
			fmt.Sprintf("stratum=%d", peerStratum),
		)
	}

	return strings.Join(parts, ", ")
}
