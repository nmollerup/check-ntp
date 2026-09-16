package main

import (
	"testing"

	"github.com/facebook/time/cmd/ntpcheck/checker"
	"github.com/facebook/time/ntp/control"
	corev2 "github.com/sensu/sensu-go/api/core/v2"
	"github.com/sensu/sensu-plugin-sdk/sensu"
	"github.com/stretchr/testify/assert"
)

func TestMain(t *testing.T) {
}

func TestCheckArgs(t *testing.T) {
	assert := assert.New(t)
	event := corev2.FixtureEvent("entity1", "check1")
	i, e := checkArgs(event)
	assert.Equal(sensu.CheckStateWarning, i)
	assert.Error(e)
	plugin.Critical = float64(20)
	i, e = checkArgs(event)
	assert.Equal(sensu.CheckStateWarning, i)
	assert.Error(e)
	plugin.Warning = float64(10)
	i, e = checkArgs(event)
	assert.Equal(sensu.CheckStateOK, i)
	assert.NoError(e)
	plugin.Critical = float64(5)
	i, e = checkArgs(event)
	assert.Equal(sensu.CheckStateWarning, i)
	assert.Error(e)

	// reset to a valid baseline before exercising root distance validation
	plugin.Warning = float64(10)
	plugin.Critical = float64(20)

	plugin.RootDistanceWarning = 100
	plugin.RootDistanceCritical = 50
	i, e = checkArgs(event)
	assert.Equal(sensu.CheckStateWarning, i)
	assert.Error(e)

	plugin.RootDistanceWarning = 50
	plugin.RootDistanceCritical = 100
	i, e = checkArgs(event)
	assert.Equal(sensu.CheckStateOK, i)
	assert.NoError(e)

	// unset (0) thresholds are always allowed, regardless of the other one
	plugin.RootDistanceWarning = 0
	plugin.RootDistanceCritical = 0
	i, e = checkArgs(event)
	assert.Equal(sensu.CheckStateOK, i)
	assert.NoError(e)

	plugin.RootDistanceWarning = 100
	plugin.RootDistanceCritical = 0
	i, e = checkArgs(event)
	assert.Equal(sensu.CheckStateOK, i)
	assert.NoError(e)

	plugin.RootDistanceWarning = 0
	plugin.RootDistanceCritical = 100
	i, e = checkArgs(event)
	assert.Equal(sensu.CheckStateOK, i)
	assert.NoError(e)

	plugin.RootDistanceWarning = 0
	plugin.RootDistanceCritical = 0
}

func sysPeer() *checker.Peer {
	return &checker.Peer{Selection: control.SelSYSPeer, Stratum: 1}
}

func baseConfig() Config {
	return Config{
		PluginConfig: sensu.PluginConfig{Name: "check-ntp"},
		Warning:      150,
		Critical:     250,
	}
}

func TestEvaluate(t *testing.T) {
	assert := assert.New(t)

	t.Run("healthy: small sys and peer offset -> OK", func(t *testing.T) {
		result := &checker.NTPCheckResult{
			SysVars: &checker.SystemVariables{Offset: 0.5, Stratum: 2},
			Peers:   map[uint16]*checker.Peer{0: sysPeer()},
		}
		stats := &checker.NTPStats{PeerOffset: 0.8, PeerJitter: 0.1, PeerStratum: 2}

		state, output := evaluate(result, stats, baseConfig())
		assert.Equal(sensu.CheckStateOK, state)
		assert.Contains(output, "OK")
		assert.Contains(output, "sys_offset=0.500000")
	})

	t.Run("VM stun regression: sys_offset ~1ms, peer_offset 350ms -> OK", func(t *testing.T) {
		result := &checker.NTPCheckResult{
			SysVars: &checker.SystemVariables{Offset: 1.0, Stratum: 2},
			Peers:   map[uint16]*checker.Peer{0: sysPeer()},
		}
		stats := &checker.NTPStats{PeerOffset: 350.0, PeerJitter: 12.0, PeerStratum: 2}

		state, output := evaluate(result, stats, baseConfig())
		assert.Equal(sensu.CheckStateOK, state, "a VM stun should not alert: system clock is fine even though peer offset spiked")
		assert.Contains(output, "OK")
		assert.Contains(output, "system offset 1.000ms within thresholds")
		assert.Contains(output, "sys_offset=1.000000")
		assert.Contains(output, "peer_offset=350.000000")
	})

	t.Run("real error: sys_offset beyond warning -> WARNING", func(t *testing.T) {
		result := &checker.NTPCheckResult{
			SysVars: &checker.SystemVariables{Offset: 200.0, Stratum: 2},
			Peers:   map[uint16]*checker.Peer{0: sysPeer()},
		}
		stats := &checker.NTPStats{PeerOffset: 200.0, PeerStratum: 2}

		state, output := evaluate(result, stats, baseConfig())
		assert.Equal(sensu.CheckStateWarning, state)
		assert.Contains(output, "WARNING")
	})

	t.Run("real error: sys_offset beyond critical -> CRITICAL", func(t *testing.T) {
		result := &checker.NTPCheckResult{
			SysVars: &checker.SystemVariables{Offset: 300.0, Stratum: 2},
			Peers:   map[uint16]*checker.Peer{0: sysPeer()},
		}
		stats := &checker.NTPStats{PeerOffset: 300.0, PeerStratum: 2}

		state, output := evaluate(result, stats, baseConfig())
		assert.Equal(sensu.CheckStateCritical, state)
		assert.Contains(output, "CRITICAL")
	})

	t.Run("negative offsets respected via absolute value", func(t *testing.T) {
		result := &checker.NTPCheckResult{
			SysVars: &checker.SystemVariables{Offset: -300.0, Stratum: 2},
			Peers:   map[uint16]*checker.Peer{0: sysPeer()},
		}
		stats := &checker.NTPStats{PeerOffset: -300.0, PeerStratum: 2}

		state, output := evaluate(result, stats, baseConfig())
		assert.Equal(sensu.CheckStateCritical, state)
		assert.Contains(output, "CRITICAL")

		result.SysVars.Offset = -200.0
		stats.PeerOffset = -200.0
		state, output = evaluate(result, stats, baseConfig())
		assert.Equal(sensu.CheckStateWarning, state)
		assert.Contains(output, "WARNING")
	})

	t.Run("unsynchronized: leap=3 -> CRITICAL", func(t *testing.T) {
		result := &checker.NTPCheckResult{
			SysVars: &checker.SystemVariables{Offset: 0.1, Stratum: 2, Leap: 3},
			Peers:   map[uint16]*checker.Peer{0: sysPeer()},
		}
		stats := &checker.NTPStats{PeerOffset: 0.1, PeerStratum: 2}

		state, output := evaluate(result, stats, baseConfig())
		assert.Equal(sensu.CheckStateCritical, state)
		assert.Contains(output, "unsynchronized (leap=3)")
	})

	t.Run("unsynchronized: no system peer -> CRITICAL", func(t *testing.T) {
		result := &checker.NTPCheckResult{
			SysVars: &checker.SystemVariables{Offset: 0.1, Stratum: 2},
			Peers:   map[uint16]*checker.Peer{},
		}

		state, output := evaluate(result, nil, baseConfig())
		assert.Equal(sensu.CheckStateCritical, state)
		assert.Contains(output, "no system peer selected")
	})

	t.Run("unsynchronized: stratum=16 -> CRITICAL", func(t *testing.T) {
		result := &checker.NTPCheckResult{
			SysVars: &checker.SystemVariables{Offset: 0.1, Stratum: 16},
			Peers:   map[uint16]*checker.Peer{0: sysPeer()},
		}
		stats := &checker.NTPStats{PeerOffset: 0.1, PeerStratum: 16}

		state, output := evaluate(result, stats, baseConfig())
		assert.Equal(sensu.CheckStateCritical, state)
		assert.Contains(output, "stratum 16 is unsynchronized")
	})

	t.Run("root distance: threshold set and breached -> alert", func(t *testing.T) {
		cfg := baseConfig()
		cfg.RootDistanceWarning = 50
		cfg.RootDistanceCritical = 100

		result := &checker.NTPCheckResult{
			SysVars: &checker.SystemVariables{Offset: 0.1, Stratum: 2, RootDelay: 40, RootDisp: 40}, // rootDistance = 20 + 40 = 60
			Peers:   map[uint16]*checker.Peer{0: sysPeer()},
		}
		stats := &checker.NTPStats{PeerOffset: 0.1, PeerStratum: 2}

		state, output := evaluate(result, stats, cfg)
		assert.Equal(sensu.CheckStateWarning, state)
		assert.Contains(output, "root distance")
		assert.Contains(output, "root_distance=60.000000")

		// bump root delay/disp so root distance crosses the critical threshold too
		result.SysVars.RootDelay = 160
		result.SysVars.RootDisp = 40 // rootDistance = 80 + 40 = 120
		state, output = evaluate(result, stats, cfg)
		assert.Equal(sensu.CheckStateCritical, state)
		assert.Contains(output, "root distance")
	})

	t.Run("root distance: threshold set and not breached -> OK, metric still present", func(t *testing.T) {
		cfg := baseConfig()
		cfg.RootDistanceWarning = 100
		cfg.RootDistanceCritical = 200

		result := &checker.NTPCheckResult{
			SysVars: &checker.SystemVariables{Offset: 0.1, Stratum: 2, RootDelay: 10, RootDisp: 10}, // rootDistance = 5 + 10 = 15
			Peers:   map[uint16]*checker.Peer{0: sysPeer()},
		}
		stats := &checker.NTPStats{PeerOffset: 0.1, PeerStratum: 2}

		state, output := evaluate(result, stats, cfg)
		assert.Equal(sensu.CheckStateOK, state)
		assert.Contains(output, "root_distance=15.000000")
	})

	t.Run("root distance: threshold unset (0) -> never alerts, metric still emitted", func(t *testing.T) {
		cfg := baseConfig() // RootDistanceWarning/Critical left at zero value

		result := &checker.NTPCheckResult{
			SysVars: &checker.SystemVariables{Offset: 0.1, Stratum: 2, RootDelay: 2000, RootDisp: 2000}, // huge root distance
			Peers:   map[uint16]*checker.Peer{0: sysPeer()},
		}
		stats := &checker.NTPStats{PeerOffset: 0.1, PeerStratum: 2}

		state, output := evaluate(result, stats, cfg)
		assert.Equal(sensu.CheckStateOK, state)
		assert.Contains(output, "root_distance=3000.000000")
		assert.NotContains(output, "root distance")
	})

	t.Run("legacy perfdata flag: old keys present only when enabled", func(t *testing.T) {
		result := &checker.NTPCheckResult{
			SysVars: &checker.SystemVariables{Offset: 1.0, Stratum: 2, ClkJitter: 0.05},
			Peers:   map[uint16]*checker.Peer{0: sysPeer()},
		}
		stats := &checker.NTPStats{PeerOffset: 350.0, PeerJitter: 12.0, PeerStratum: 3}

		cfg := baseConfig()
		_, output := evaluate(result, stats, cfg)
		assert.NotContains(output, ", offset=350.000000")
		assert.NotContains(output, ", stratum=3")

		cfg.LegacyPerfdata = true
		_, output = evaluate(result, stats, cfg)
		// new, system-based fields keep their meaning
		assert.Contains(output, "sys_offset=1.000000")
		assert.Contains(output, "stratum=2")
		assert.Contains(output, "clk_jitter=0.050000")
		// legacy, peer-based fields are appended with the old names/meaning
		assert.Contains(output, "offset=350.000000")
		assert.Contains(output, "clk_jitter=12.000000")
		assert.Contains(output, "stratum=3")
	})
}
