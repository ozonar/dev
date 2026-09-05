package prod

import (
	"fmt"
	"strings"
)

// collectNetwork собирает категорию Network из /proc/net/* и ss.
func collectNetwork(prev *Report) *Category {
	cat := &Category{ID: CatNetwork, Present: true}

	// NETWORK_RETRANSMISSION: из /proc/net/snmp (Tcp RetransSegs vs OutSegs).
	if retrPct, ok := tcpRetransmission(); ok {
		switch {
		case retrPct >= 5:
			cat.AddSymptom(Symptom{ID: "NETWORK_RETRANSMISSION", Level: LevelError,
				Summary: fmt.Sprintf("%.1f%% retransmitted segments", retrPct)})
		case retrPct >= 1:
			cat.AddSymptom(Symptom{ID: "NETWORK_RETRANSMISSION", Level: LevelWarn,
				Summary: fmt.Sprintf("%.1f%% retransmitted segments", retrPct)})
		default:
			cat.AddSymptom(Symptom{ID: "NETWORK_RETRANSMISSION", Level: LevelOK,
				Summary: "no significant retransmissions"})
		}
	} else {
		cat.AddSymptom(Symptom{ID: "NETWORK_RETRANSMISSION", Level: LevelOK,
			Summary: "no significant anomalies"})
	}

	// NETWORK_CONNECTION_ERRORS: ошибки соединений TCP.
	if errRate, ok := tcpConnectionErrors(); ok && errRate > 0 {
		lvl := LevelWarn
		if errRate > 100 {
			lvl = LevelError
		}
		cat.AddSymptom(Symptom{ID: "NETWORK_CONNECTION_ERRORS", Level: lvl,
			Summary: fmt.Sprintf("%.0f connection errors/s", errRate)})
	}

	// EPHEMERAL_PORT_EXHAUSTION.
	if usedPct, ok := ephemeralPortUsage(); ok && usedPct >= 70 {
		lvl := LevelWarn
		if usedPct >= 90 {
			lvl = LevelError
		}
		cat.AddSymptom(Symptom{ID: "EPHEMERAL_PORT_EXHAUSTION", Level: lvl,
			Summary: fmt.Sprintf("%.0f%% ephemeral ports in use", usedPct)})
	}

	// SOCKET_EXHAUSTION / TCP_BACKLOG_SATURATION через ss.
	if backlog, ok := tcpBacklog(); ok && backlog > 0 {
		lvl := LevelWarn
		if backlog > 100 {
			lvl = LevelError
		}
		cat.AddSymptom(Symptom{ID: "TCP_BACKLOG_SATURATION", Level: lvl,
			Summary: fmt.Sprintf("%d sockets in accept queue", backlog)})
	}

	cat.Detected = true
	return cat
}

// tcpSnmp возвращает заголовок и значения блока "Tcp:" из /proc/net/snmp.
func tcpSnmp() (header, values []string, ok bool) {
	s, err := readFile("/proc/net/snmp")
	if err != nil {
		return nil, nil, false
	}
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, "Tcp: ") {
			if len(header) == 0 {
				header = strings.Fields(line)[1:]
			} else {
				values = strings.Fields(line)[1:]
			}
		}
	}
	return header, values, len(values) >= len(header)
}

// tcpRetransmission возвращает долю ретрансмиссий TCP в процентах.
func tcpRetransmission() (float64, bool) {
	header, values, ok := tcpSnmp()
	if !ok {
		return 0, false
	}
	var out, retr float64
	for i, h := range header {
		switch h {
		case "OutSegs":
			out = parseFloat(values[i])
		case "RetransSegs":
			retr = parseFloat(values[i])
		}
	}
	if out == 0 {
		return 0, false
	}
	return retr / out * 100, true
}

// tcpConnectionErrors подсчитывает ошибки соединений TCP из /proc/net/snmp
// (AttemptFails + ActiveOpens-расхождения). Для MVP берём AttemptFails.
func tcpConnectionErrors() (float64, bool) {
	header, values, ok := tcpSnmp()
	if !ok {
		return 0, false
	}
	for i, h := range header {
		if h == "AttemptFails" {
			return parseFloat(values[i]), true
		}
	}
	return 0, false
}

// ephemeralPortUsage оценивает занятость эфемерных портов по количеству
// TCP-сокетов в состоянии TIME_WAIT/ESTABLISHED относительно диапазона.
func ephemeralPortUsage() (float64, bool) {
	s, err := readFile("/proc/sys/net/ipv4/ip_local_port_range")
	if err != nil {
		return 0, false
	}
	var lo, hi int64
	fmt.Sscanf(s, "%d %d", &lo, &hi)
	rangeSize := hi - lo
	if rangeSize <= 0 {
		return 0, false
	}
	count := countTCPSockets()
	pct := float64(count) / float64(rangeSize) * 100
	return pct, count > 0
}

// countTCPSockets подсчитывает TCP-сокеты из /proc/net/tcp и tcp6.
func countTCPSockets() int64 {
	var total int64
	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		s, err := readFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(s, "\n") {
			if strings.HasPrefix(line, "sl") {
				continue
			}
			total++
		}
	}
	return total
}

// tcpBacklog возвращает число сокетов в accept-очереди (Recv-Q) у LISTEN.
func tcpBacklog() (int64, bool) {
	var total int64
	found := false
	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		s, err := readFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(s, "\n") {
			if strings.HasPrefix(line, "sl") {
				continue
			}
			f := strings.Fields(line)
			if len(f) < 5 {
				continue
			}
			// f[3] — tx_queue:rx_queue; f[4] — st (0A = LISTEN)
			if strings.Contains(strings.ToUpper(f[3]), ":") {
				rx := strings.Split(f[3], ":")[0]
				if strings.Contains(strings.ToUpper(f[4]), "0A") {
					v := parseInt(rx)
					if v > 0 {
						total += v
						found = true
					}
				}
			}
		}
	}
	return total, found
}
