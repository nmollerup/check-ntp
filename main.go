package main

import (
	"fmt"
	"math"

	"github.com/facebookincubator/ntp/ntpcheck/checker"
	corev2 "github.com/sensu/sensu-go/api/core/v2"
	"github.com/sensu/sensu-plugin-sdk/sensu"
)

// Config represents the check plugin config.
type Config struct {
	sensu.PluginConfig
	Warning  float64
	Critical float64
	Address  string
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
			Usage:     "Critical threshold for offset in ms",
			Value:     &plugin.Critical,
		},
		&sensu.PluginConfigOption[float64]{
			Path:      "warning",
			Argument:  "warning",
			Shorthand: "w",
			Default:   float64(10),
			Usage:     "Warning threshold for offset in ms",
			Value:     &plugin.Warning,
		},
		&sensu.PluginConfigOption[string]{
			Path:      "address",
			Argument:  "address",
			Shorthand: "a",
			Default:   "127.0.0.1:123",
			Usage:     "Address to check NTP",
			Value:     &plugin.Address,
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
	return sensu.CheckStateOK, nil
}

func executeCheck(event *corev2.Event) (int, error) {
	result, err := checker.RunCheck(plugin.Address)
	if err != nil {
		fmt.Printf("%s CRITICAL: failed to run check, error: %v\n", plugin.PluginConfig.Name, err)
		return sensu.CheckStateCritical, nil
	}
	stats, err := checker.NewNTPStats(result)
	if err != nil {
		fmt.Printf("%s CRITICAL: failed to extract NTP statistics, error: %v\n", plugin.PluginConfig.Name, err)
		return sensu.CheckStateCritical, nil
	}
	perfData := fmt.Sprintf("clk_jitter=%f, clk_wander=%f, frequency=%f, mintc=%d, offset=%f, stratum=%d, sys_jitter=%f, tc=%d", stats.PeerJitter, result.SysVars.ClkWander, stats.Frequency, result.SysVars.MinTC, stats.PeerOffset, stats.PeerStratum, result.SysVars.SysJitter, result.SysVars.TC)

	if math.Abs(stats.PeerOffset) > plugin.Critical {
		fmt.Printf("%s CRITICAL: offset %.3f exceeds threshold  | %s\n", plugin.PluginConfig.Name, stats.PeerOffset, perfData)
		return sensu.CheckStateCritical, nil
	} else if math.Abs(stats.PeerOffset) > plugin.Warning {
		fmt.Printf("%s WARNING: offset %.3f exceeds threshold | %s\n", plugin.PluginConfig.Name, stats.PeerOffset, perfData)
		return sensu.CheckStateWarning, nil
	}
	fmt.Printf("%s OK: offset %.3f within thresholds | %s\n", plugin.PluginConfig.Name, stats.PeerOffset, perfData)
	return sensu.CheckStateOK, nil
}
