package pvdata

import "time"

var ntTypes = []TypeIDer{
	Enum{},
	Time{},
	Alarm{},
	AlarmLimit{},
	ValueAlarm{},
	Display{},
	Control{},
}

type Enum struct {
	Index   PVInt    `pvaccess:"index"`
	Choices []string `pvaccess:"choices"`
}

func (Enum) TypeID() string {
	return "enum_t"
}

type Time struct {
	Time    time.Time
	UserTag PVInt
}

func (Time) TypeID() string {
	return "time_t"
}
func (t Time) PVEncode(s *EncoderState) error {
	secondsPastEpoch := PVLong(t.Time.Unix())
	nanoseconds := PVInt(t.Time.Nanosecond())
	return Encode(s, &secondsPastEpoch, &nanoseconds, &t.UserTag)
}
func (t *Time) PVDecode(s *DecoderState) error {
	var secondsPastEpoch PVLong
	var nanoseconds PVInt
	if err := Decode(s, &secondsPastEpoch, &nanoseconds, &t.UserTag); err != nil {
		return err
	}
	t.Time = time.Unix(int64(secondsPastEpoch), int64(nanoseconds))
	return nil
}

type Alarm struct {
	Severity PVInt    `pvaccess:"severity"`
	Status   PVInt    `pvaccess:"status"`
	Message  PVString `pvaccess:"message"`
}

func (Alarm) TypeID() string {
	return "alarm_t"
}

type AlarmLimit struct {
	Active           PVBoolean `pvaccess:"active"`
	LowAlarmLimit    PVDouble  `pvaccess:"lowAlarmLimit"`
	LowWarningLimit  PVDouble  `pvaccess:"lowWarningLimit"`
	HighWarningLimit PVDouble  `pvaccess:"highWarningLimit"`
	HighAlarmLimit   PVDouble  `pvaccess:"highAlarmLimit"`

	LowAlarmSeverity    PVInt `pvaccess:"lowAlarmSeverity"`
	LowWarningSeverity  PVInt `pvaccess:"lowWarningSeverity"`
	HighWarningSeverity PVInt `pvaccess:"highWarningSeverity"`
	HighAlarmSeverity   PVInt `pvaccess:"highAlarmSeverity"`

	Hysteresis PVDouble `pvaccess:"hysteresis"`
}

func (AlarmLimit) TypeID() string {
	return "alarmLimit_t"
}

// valueAlarm_t is used by older PVAccess servers.
// TODO: The limit fields of ValueAlarm can be any integer or floating point type, not just doubles.
type ValueAlarm AlarmLimit

func (ValueAlarm) TypeID() string {
	return "valueAlarm_t"
}

type Display struct {
	LimitLow    PVDouble `pvaccess:"limitLow"`
	LimitHigh   PVDouble `pvaccess:"limitHigh"`
	Description PVString `pvaccess:"description"`
	Units       PVString `pvaccess:"units"`
	Precision   PVInt    `pvaccess:"precision"`
	Form        Enum     `pvaccess:"enum"`
}

func (Display) TypeID() string {
	return "display_t"
}

type Control struct {
	LimitLow  PVDouble `pvaccess:"limitLow"`
	LimitHigh PVDouble `pvaccess:"limitHigh"`
	MinStep   PVDouble `pvaccess:"minStep"`
}

func (Control) TypeID() string {
	return "control_t"
}
