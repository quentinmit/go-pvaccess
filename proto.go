package pvaccess

import (
	"encoding/binary"
	"fmt"

	"github.com/quentinmit/go-pvaccess/pvdata"
)

// TODO: Messages MUST be aligned on a 64-bit boundary (Pad with zeros???)

type pvAccessHeader struct {
	Magic          pvdata.PVByte
	Version        pvdata.PVByte
	Flags          pvdata.PVByte
	MessageCommand pvdata.PVByte
	PayloadSize    pvdata.PVInt
}

func (v *pvAccessHeader) PVEncode(s *EncoderState) error {
	if err := v.Magic.PVEncode(s); err != nil {
		return err
	}
	if err := v.Version.PVEncode(s); err != nil {
		return err
	}
	if err := v.Flags.PVEncode(s); err != nil {
		return err
	}
	return v.PayloadSize.PVEncode(s)
}
func (v *pvAccessHeader) PVDecode(s *EncoderState) error {
	if err := v.Magic.PVDecode(s); err != nil {
		return err
	}
	if v.Magic != 0xCA {
		return fmt.Errorf("unexpected magic %x", v.Magic)
	}
	if err := v.Version.PVDecode(s); err != nil {
		return err
	}
	if err := v.Flags.PVDecode(s); err != nil {
		return err
	}
	if err := v.MessageCommand.PVDecode(s); err != nil {
		return err
	}
	// Need to decode flags before decoding PayloadSize
	// TODO: Ignore these flags if Set byte order message told us to
	if v.Flags & 0x70 {
		s.ByteOrder = binary.BigEndian
	} else {
		s.ByteOrder = binary.LittleEndian
	}
	return v.PayloadSize.PVDecode(s)
}

type beaconMessage struct {
	GUID             [12]byte
	Flags            byte
	BeaconSequenceID byte
	ChangeCount      int16
	ServerAddress    [16]byte
	ServerPort       uint16
	Protocol         string
	ServerStatus     PVAny
}
