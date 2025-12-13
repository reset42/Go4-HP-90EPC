package reader

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/tarm/serial"

	"hp90epc/logging"
	"hp90epc/model"
)

type LatestSetter interface {
	Set(*model.Measurement)
}

type Logger interface {
	Push(*model.Measurement)
}

func RunLoop(
	ctx context.Context,
	port string,
	baud int,
	latest LatestSetter,
	logger Logger,
	onFrameOK func(),
) error {
	// reconnect loop
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		c := &serial.Config{
			Name: port,
			Baud: baud,
			// Blockierend lesen: wir verlassen uns auf Close() beim Stop/Ctx-Cancel
			ReadTimeout: 0,
		}

		s, err := serial.OpenPort(c)
		if err != nil {
			// Port nicht da → kurz warten und retry
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(600 * time.Millisecond):
				continue
			}
		}

		// read loop (stream parser, no blocking "exactly 14 bytes")
		err = func() error {
			defer s.Close()

			frame := make([]byte, 14)
			idx := 0
			tmp := make([]byte, 256)
			frames := 0
			zeroReads := 0
			resyncs := 0
			lastLog := time.Now()

			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}

				n, err := s.Read(tmp)
				if err != nil {
					// bei Timeout etc. weiter, bei echten Errors raus
					// tarm/serial liefert meist plain error strings – wir treaten alles als reconnect-worthy
					return err
				}

				if n == 0 {
					// Timeout -> wir bleiben im Loop
					zeroReads++
					continue
				}

				for i := 0; i < n; i++ {
					b := tmp[i]
					want := byte((idx + 1) << 4) // idx=0 -> 0x10, ... idx=13 -> 0xE0

					if (b & 0xF0) == want {
						frame[idx] = b
						idx++
						if idx == 14 {
							// Frame komplett
							m := decodeFrame(frame)
							if m != nil {
								if latest != nil {
									latest.Set(m)
								}
								if logger != nil {
									logger.Push(m)
								}
								if onFrameOK != nil {
									onFrameOK()
								}
								frames++
							}
							idx = 0
						}
						continue
					}

					// mismatch: resync
					resyncs++
					if (b & 0xF0) == 0x10 {
						// Byte könnte Start eines neuen Frames sein
						frame[0] = b
						idx = 1
					} else {
						idx = 0
					}
				}

				if time.Since(lastLog) >= time.Second {
					log.Printf("reader: fps=%d zero_reads=%d resyncs=%d idx=%d", frames, zeroReads, resyncs, idx)
					frames = 0
					zeroReads = 0
					resyncs = 0
					lastLog = time.Now()
				}
			}
		}()

		// wenn ctx → Ende; sonst reconnect
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// kleiner backoff
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(400 * time.Millisecond):
		}
	}
}

// ===== Helpers (Frame + Decode) =====

func parseDigit(b byte) int {
	b &^= 1 << 7
	switch b {
	case 0x7d:
		return 0
	case 0x05:
		return 1
	case 0x5b:
		return 2
	case 0x1f:
		return 3
	case 0x27:
		return 4
	case 0x3e:
		return 5
	case 0x7e:
		return 6
	case 0x15:
		return 7
	case 0x7f:
		return 8
	case 0x3f:
		return 9
	default:
		return -1
	}
}

func decodeFrame(b []byte) *model.Measurement {
	if len(b) != 14 {
		return nil
	}

	// Sign
	sign := 1.0
	if b[1]&(1<<3) != 0 {
		sign = -1.0
	}

	// Digits
	digitBytes := make([]byte, 4)
	digits := make([]int, 4)
	numeric := true

	for i := 0; i < 4; i++ {
		hi := b[1+2*i] & 0x0F
		lo := b[1+2*i+1] & 0x0F
		db := (hi << 4) | lo
		digitBytes[i] = db
		d := parseDigit(db)
		if d < 0 {
			numeric = false
		}
		digits[i] = d
	}

	intval := 0
	if numeric {
		for i := 0; i < 4; i++ {
			intval = intval*10 + digits[i]
		}
	}

	// Decimal point
	div := 1.0
	switch {
	case b[3]&(1<<3) != 0:
		div = 1000.0
	case b[5]&(1<<3) != 0:
		div = 100.0
	case b[7]&(1<<3) != 0:
		div = 10.0
	}

	floatval := float64(intval) / div
	floatval *= sign

	// Prefix flags
	isNano := b[9]&(1<<2) != 0
	isMicro := b[9]&(1<<3) != 0
	isKilo := b[9]&(1<<1) != 0
	isMilli := b[10]&(1<<3) != 0
	isMega := b[10]&(1<<1) != 0

	if isNano {
		floatval /= 1e9
	}
	if isMicro {
		floatval /= 1e6
	}
	if isMilli {
		floatval /= 1e3
	}
	if isKilo {
		floatval *= 1e3
	}
	if isMega {
		floatval *= 1e6
	}

	// Mode + flags
	isAC := b[0]&(1<<3) != 0
	isDC := b[0]&(1<<2) != 0
	auto := b[0]&(1<<1) != 0

	isPercent := b[10]&(1<<2) != 0
	isFarad := b[11]&(1<<3) != 0
	isOhm := b[11]&(1<<2) != 0
	isRel := b[11]&(1<<1) != 0
	isHold := b[11]&(1<<0) != 0

	isAmp := b[12]&(1<<3) != 0
	isVolt := b[12]&(1<<2) != 0
	isHz := b[12]&(1<<1) != 0
	lowBatt := b[12]&(1<<0) != 0

	// °C: bei deinen Beispielen nur bei ... E4 (low nibble bit2)
	isCelsius := (b[13] & 0x04) != 0

	mode := ""
	if isAC {
		mode = "AC"
	} else if isDC {
		mode = "DC"
	}

	// Unit base
	unit := ""
	switch {
	case isCelsius:
		unit = "°C"
		mode = "" // Temperatur hat kein AC/DC
	case isPercent:
		unit = "%"
	case isFarad:
		unit = "F"
	case isOhm:
		unit = "Ohm"
	case isAmp:
		unit = "A"
	case isVolt:
		unit = "V"
	case isHz:
		unit = "Hz"
	}

	// Prefix string (Fix: mV statt MV, µ statt u)
	prefix := ""
	switch {
	case isMega:
		prefix = "M"
	case isKilo:
		prefix = "k"
	case isMilli:
		prefix = "m"
	case isMicro:
		prefix = "µ"
	case isNano:
		prefix = "n"
	}

	fullUnit := unit
	if unit != "" && unit != "%" && unit != "°C" {
		fullUnit = prefix + unit
	}

	// Normalize some unit strings for display (mV, µA etc. already handled by prefix; percent and celsius stay as-is)
	if fullUnit == "MV" {
		fullUnit = "mV"
	}

	// ValueStr
	valueStr := "????"
	if numeric {
		s := fmt.Sprintf("%d%d%d%d", digits[0], digits[1], digits[2], digits[3])
		switch {
		case b[3]&(1<<3) != 0:
			s = fmt.Sprintf("%c.%c%c%c", s[0], s[1], s[2], s[3])
		case b[5]&(1<<3) != 0:
			s = fmt.Sprintf("%c%c.%c%c", s[0], s[1], s[2], s[3])
		case b[7]&(1<<3) != 0:
			s = fmt.Sprintf("%c%c%c.%c", s[0], s[1], s[2], s[3])
		}
		if sign < 0 {
			s = "-" + s
		}
		valueStr = s
	}

	var valPtr *float64
	if numeric {
		v := floatval
		valPtr = &v
	}

	// Raw hex
	var sb strings.Builder
	for i, x := range b {
		if i > 0 {
			sb.WriteByte(' ')
		}
		fmt.Fprintf(&sb, "%02X", x)
	}

	return &model.Measurement{
		Value:    valPtr,
		ValueStr: valueStr,
		Unit:     fullUnit,
		Mode:     mode,
		Auto:     auto,
		Hold:     isHold,
		Rel:      isRel,
		LowBatt:  lowBatt,
		RawHex:   sb.String(),
	}
}

// (optional) wenn du später Unit-Tests willst:
var _ = logging.LogStatus{}
